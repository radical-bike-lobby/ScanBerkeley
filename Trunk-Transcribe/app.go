package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/slack-go/slack"
	"golang.org/x/sync/errgroup"
)

var r2Key string = os.Getenv("CLOUDFLARE_R2_KEY")
var r2Secret string = os.Getenv("CLOUDFLARE_R2_SECRET")
var r2Path string = "https://pub-85c4b9a9667540e99c0109c068c47e0f.r2.dev"

// slack setup
var slackapiSecret string = os.Getenv("SLACK_API_SECRET")
var slackapiSecretSecondary string = os.Getenv("SLACK_API_SECRET_SECONDARY")

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

type Config struct {
	uploader             *s3manager.Uploader
	slackClient          *slack.Client
	slackClientSecondary *slack.Client
}

var dedupeCache *lru.Cache[string, bool]
var location *time.Location

func init() {
	var err error
	location, err = time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}

	dedupeCache, err = lru.New[string, bool](1000)
	if err != nil {
		panic(err)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"

	}

	var api, secondary *slack.Client
	if slackapiSecret == "" {
		log.Fatal("Missing SLACK_API_SECRET")
	} else {
		api = slack.New(slackapiSecret)
	}

	if slackapiSecretSecondary != "" {
		secondary = slack.New(slackapiSecretSecondary)
	} else {
		log.Fatal("Missing SLACK_API_SECRET_SECONDARY")
	}

	// R2 setup
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cloudflareAccountID)
	fmt.Println("Using cloudflare R2 endpoint: ", endpoint)

	r2Config := &aws.Config{
		Region:      aws.String("auto"),
		Credentials: credentials.NewStaticCredentials(r2Key, r2Secret, ""),
		Endpoint:    aws.String(endpoint),
	}
	uploader := s3manager.NewUploader(session.New(r2Config))

	config := &Config{
		uploader:             uploader,
		slackClient:          api,
		slackClientSecondary: secondary,
	}

	ch := make(chan *TranscriptionRequest)
	var wg sync.WaitGroup

	// start transcription request goroutine pool
	go func() {

		for req := range ch {
			var requestsInFlight atomic.Int64
			wg.Add(1)
			requestsInFlight.Add(1)
			fmt.Println("Requests in flight: ", requestsInFlight.Load())
			go func() {
				ctx := context.Background()
				handleTranscriptionRequest(ctx, config, req)
				wg.Done()
				requestsInFlight.Add(-1)
			}()
		}
	}()

	// create server to serve http requests
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux(config, ch),
	}

	log.Println("Starting server on port: ", port)

	// start server
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections.")
	}()

	// handle sigint
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	}

	// close TranscriptionRequest channel
	close(ch)

	// wait for transcription request go routines to complete
	wg.Wait()

	log.Println("Graceful shutdown complete.")
}

// mux creates a new ServeMux router
func mux(config *Config, ch chan *TranscriptionRequest) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
		}
		t.ExecuteTemplate(w, "index.html.tmpl", data)
	})

	mux.HandleFunc("/audio", func(w http.ResponseWriter, r *http.Request) {
		link := r.URL.Query()["link"]
		if len(link) < 1 {
			writeErr(w, errors.New("link not provided"))
			return
		}

		result, err := url.JoinPath(r2Path, link[0])
		if err != nil {
			writeErr(w, err)
			return
		}
		data := map[string]string{
			"Link": result,
		}
		t.ExecuteTemplate(w, "audio.html.tmpl", data)
	})

	mux.HandleFunc("/transcribe", func(w http.ResponseWriter, r *http.Request) {
		defer http.ServeContent(w, r, "", time.Now(), strings.NewReader("ok"))

		r.Body = http.MaxBytesReader(w, r.Body, 20<<20+512)
		req, err := createTranscriptionRequestFromTrunkRecorder(r.Context(), config, r)
		if err != nil {
			log.Println("Error creating transcription request: ", err.Error())
			writeErr(w, err)
			return
		}
		ch <- req
	})

	mux.HandleFunc("/transcribe/api/call-upload", func(w http.ResponseWriter, r *http.Request) {
		defer http.ServeContent(w, r, "", time.Now(), strings.NewReader("ok"))

		r.Body = http.MaxBytesReader(w, r.Body, 20<<20+512)
		req, err := createTranscriptionRequestFromRdio(r.Context(), config, r)
		if err != nil {
			log.Println("Error creating transcription request: ", err.Error())
			return
		}
		ch <- req
	})

	return mux
}

func createTranscriptionRequestFromRdio(ctx context.Context, config *Config, r *http.Request) (*TranscriptionRequest, error) {

	var call Call
	var key string
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	mr := multipart.NewReader(r.Body, params["boundary"])

loop:
	for {
		p, err := mr.NextPart()
		switch err {
		case io.EOF:
			break loop
		case nil:
		default:
			return nil, err
		}

		b, err := io.ReadAll(p)
		if err != nil {
			return nil, err
		}

		switch p.FormName() {
		case "key":
			key = string(b)
			if key != "1" {
				return nil, errors.New("Wrong api key received from rdio upstream")
			}
		default:
			call.ParseMultipartContent(p, b)
		}
	}

	if ok, err := call.IsValid(); !ok {
		return nil, err
	}

	metadata, err := call.ToMetadata()
	if err != nil {
		return nil, err
	}

	resolved := channelResolver(metadata)
	var channels []SlackChannelID
	for _, channel := range resolved {
		if slices.Contains(BERKELEY_CHANNELS, channel) { // filter out Berkeley channels
			continue
		}
		channels = append(channels, channel)
	}

	request := &TranscriptionRequest{
		Filename:      call.AudioName,
		Data:          call.Audio,
		Meta:          metadata,
		SlackChannels: channels,
		UploadToRdio:  false,
	}

	data, _ := call.ToJson()
	log.Printf("Successful parse of rdio request: %s", data)

	return request, nil

}
func createTranscriptionRequestFromTrunkRecorder(ctx context.Context, config *Config, r *http.Request) (*TranscriptionRequest, error) {

	err := r.ParseMultipartForm(1 << 20)
	if err != nil {
		return nil, err
	}

	js, _, err := r.FormFile("call_json")
	if err != nil {
		return nil, err
	}
	defer js.Close()
	callJson, _ := ioutil.ReadAll(js)

	file, header, err := r.FormFile("call_audio")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	data, _ := ioutil.ReadAll(file)

	var metadata Metadata
	err = json.Unmarshal(callJson, &metadata)
	if err != nil {
		return nil, err
	}

	resolved := channelResolver(metadata)
	var channels []SlackChannelID
	for _, channel := range resolved {
		if !slices.Contains(BERKELEY_CHANNELS, channel) { // filter out Berkeley channels
			continue
		}
		channels = append(channels, channel)
	}

	return &TranscriptionRequest{
		Filename:      filename,
		Data:          data,
		Meta:          metadata,
		SlackChannels: channels,
		UploadToRdio:  true,
	}, nil
}

// handleTranscriptionRequest
func handleTranscriptionRequest(ctx context.Context, config *Config, req *TranscriptionRequest) error {
	var err error
	start := time.Now()
	log.Println("Handling transcription request: ", req.Filename)

	defer func() {
		duration := time.Now().Sub(start)
		if err != nil {
			log.Printf("Faileds transcription request [%v]: %s : %v", duration, req.Filename, err)
		} else {
			log.Printf("Finished transcription request [%v]: %s", duration, req.Filename)
		}
	}()

	enhanceCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	enhanced, enhanceErr := io.ReadAll(removeSilence(enhanceCtx, req.Data))
	// enhanced, enhanceErr := deepFilter(enhanceCtx, req.Data)

	if enhanceErr != nil {
		log.Println("[handleTranscriptionRequest] Error performing enhancement on audio. Falling back to original. ", enhanceErr)
	} else {
		req.Data = enhanced
	}

	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		_, err = transcribeAndUpload(ctx, config, req)
		wg.Done()
		if err != nil {
			fmt.Println("[handleTranscriptionRequest]Error transcribing and uploading to slack: ", err.Error())
		}
	}()

	go func() {
		wg.Add(1)
		err = uploadToRdio(ctx, req)
		wg.Done()
		if err != nil {
			fmt.Println("[handleTranscriptionRequest] Error uploading to rdio: ", err.Error())
		}
	}()

	wg.Wait()
	return nil
}

// dedupeDispatch checks if the specified dispatch (described by its metadata) has already been seen.
// Returns true if the dispatch is a duplicate, false otherwise.
func dedupeDispatch(meta Metadata) (dupe bool) {

	// construct a cache key consisting of all the srcs (parties in the call),
	// the talkgroup, and the startime
	var srcs string
	for _, src := range meta.SrcList {
		srcs += fmt.Sprintf(".%v", src.Src)
	}

	startTime := meta.StartTime - (meta.StartTime % 5) // time to the nearest 5 second increment
	dedupeKey := fmt.Sprintf("tg.%d.start.%d.srcs%s", meta.Talkgroup, startTime, srcs)

	// atomically check-or-set. Return whether the key already existed.
	exists, _ := dedupeCache.ContainsOrAdd(dedupeKey, true)

	return exists
}

// transcribeAndUpload transcribes the audio to text, posts the text to slack and persists the audio file to S3,
func transcribeAndUpload(ctx context.Context, config *Config, req *TranscriptionRequest) (string, error) {

	if len(req.SlackChannels) == 0 {
		return "", nil
	}
	key := req.FilePath()
	data := req.Data
	metadata := req.Meta

	msg, segments, err := whisper(ctx, req.Data)

	if err == nil {
		fmt.Println(key+": ", msg)
	} else {
		msg = "Error transcribing text: " + err.Error()
	}

	metadata.AudioText = msg
	metadata.Segments = segments
	metadata.URL = fmt.Sprintf("https://trunk-transcribe.fly.dev/audio?link=%s", key)

	wg, gctx := errgroup.WithContext(ctx)

	//upload to Cloudflare R2 (with s3 compatible api)
	wg.Go(func() error {
		return uploadS3(gctx, config.uploader, key, bytes.NewReader(data), metadata)
	})

	wg.Go(func() error {
		return postToSlack(gctx, config, req.SlackChannels, key, data, metadata)
	})

	err = wg.Wait()
	return msg, err
}

// transcribeAndUpload uploads the audio to S3
func uploadS3(ctx context.Context, uploader *s3manager.Uploader, key string, reader io.Reader, meta Metadata) error {

	s3Meta := make(map[string]*string)
	s3Meta["short-name"] = aws.String(meta.ShortName)
	s3Meta["call-length"] = aws.String(strconv.FormatInt(meta.CallLength, 10))
	s3Meta["talk-group"] = aws.String(strconv.FormatInt(meta.Talkgroup, 10))
	s3Meta["priority"] = aws.String(strconv.FormatInt(meta.Priority, 10))

	input := &s3manager.UploadInput{
		Bucket:      aws.String("scanner-berkeley"),         // bucket's name
		Key:         aws.String(key),                        // files destination location
		Body:        reader,                                 // content of the file
		Metadata:    s3Meta,                                 // metadata
		ContentType: aws.String("application/octet-stream"), // content type
	}
	_, err := uploader.UploadWithContext(ctx, input)
	return err
}

func postToSlack(ctx context.Context, config *Config, channelIDs []SlackChannelID, key string, data []byte, meta Metadata) error {

	if len(channelIDs) == 0 {
		return nil
	}

	if meta.AudioText == "" {
		meta.AudioText = "Could not transcribe audio"
	}

	blocks := meta.Segments

	for i, block := range blocks {
		if i >= len(meta.SrcList) {
			break
		} else if len(block) == 0 {
			continue
		}

		src := meta.SrcList[i]
		tag := src.Tag
		if tag == "" {
			tag = strconv.FormatInt(src.Src, 10)
		}
		block = strings.TrimSpace(block)
		blocks[i] = tag + ": " + block
	}

	// Talkgroup
	// Transcription
	// Audio link
	// Audio info: length, date
	// Mentions

	blocks = append([]string{"*" + meta.TalkgroupTag + "* | _" + meta.TalkGroupDesc + "_"}, blocks...)
	blocks = append(blocks, fmt.Sprintf("<%s|Audio>", meta.URL))
	blocks = append(blocks, fmt.Sprintf("%d seconds | %s", meta.CallLength, time.Now().In(location).Format("Mon, Jan 02 2006 3:04PM MST")))

	for _, channelID := range channelIDs {

		client := config.slackClientSecondary
		if slices.Contains(PRIMARY_CHANNELS, channelID) {
			client = config.slackClient
		}

		slackMeta := ExtractSlackMeta(meta, channelID, notifsMap)
		mentions := slackMeta.Mentions
		message := blocks
		if str := strings.Join(mentions, " "); len(str) > 0 {
			message = append(message, str)
		}

		sentences := strings.Join(message, "\n")

		// upload audio
		_, err := client.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
			Filename:       filepath.Base(key),
			FileSize:       len(data),
			Reader:         bytes.NewReader(data),
			InitialComment: sentences,
			Channel:        string(channelID),
		})
		if err != nil {
			log.Println("Error uploading file to slack: ", err)
			return err
		}

	}

	return nil
}

// uploadToRdio uploads the audio file to the radio interface: https://rdio-eastbay.fly.dev
func uploadToRdio(ctx context.Context, req *TranscriptionRequest) error {

	if !req.UploadToRdio {
		return nil
	}

	filename, meta, reader := req.Filename, req.Meta, bytes.NewReader(req.Data)

	rdioScannerSecret := os.Getenv("RDIO_SCANNER_API_KEY")
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	defer writer.Close()

	callJson, _ := json.Marshal(meta)
	writer.WriteField("key", rdioScannerSecret)
	writer.WriteField("meta", string(callJson))
	writer.WriteField("system", "1000") // "eastbay" system
	part, _ := writer.CreateFormFile("audio", filename)

	io.Copy(part, reader)
	writer.Close()

	uri := "https://rdio-eastbay.fly.dev/api/trunk-recorder-call-upload"
	res, err := http.Post(uri, writer.FormDataContentType(), body)
	if err != nil {
		return err
	}
	resBody, _ := io.ReadAll(res.Body)
	defer res.Body.Close()

	if res.StatusCode > 299 {
		return fmt.Errorf("Error uploading to rdio-scanner: Response failed with status code: %d and\nbody: %s\n", res.StatusCode, resBody)
	}
	return nil
}

// ExtractSlackMeta returns the list of mentions and an address to append corresponding to matching keywords in the
// sentance
// It accepts a sentace to match keywords against. The keywords map provides a map
// of mentions to keywords to match.

func ExtractSlackMeta(meta Metadata, channelID SlackChannelID, notifsMap map[SlackUserID][]Notifs) (slackMeta SlackMeta) {

	text := strings.ToLower(meta.AudioText)
	words := wordsRegex.FindAllString(text, -1) //split text into words array

	talkgroupID := TalkGroupID(meta.Talkgroup)

	for userID, notifs := range notifsMap {
		for _, notif := range notifs {
			if notif.MatchesText(channelID, talkgroupID, text, words) {
				slackMeta.Mentions = append(slackMeta.Mentions, "<@"+string(userID)+">")
				break
			}
		}
	}

	// match address
	for i := 0; i < len(words); i += 1 {
		prefix := words[i]
		var word string
		if len(words[i:]) >= 2 {
			word = words[i+1]
		}

		hasAddrNumber := numericRegex.MatchString(prefix)

		for _, street := range streets {
			lowerCase := strings.ToLower(street)
			found := false
			switch {
			case hasAddrNumber && word == lowerCase:
				slackMeta.Address.PrimaryAddress = prefix + " " + street
				// incr index by one to move on to the next pair
				i += 1
				found = true
			case prefix == lowerCase:
				slackMeta.Address = slackMeta.Address.AppendStreet(street)
				found = true
			}
			if found {
				break
			}
		}

	}

	return slackMeta
}

func writeErr(w http.ResponseWriter, err error) {
	fmt.Println("Error: ", err.Error())
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
