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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/rakyll/openai-go"
	"github.com/rakyll/openai-go/audio"
	"github.com/slack-go/slack"
	"golang.org/x/sync/errgroup"
)

var (
	puncRegex = regexp.MustCompile("[\\.\\!\\?;]\\s+")
	streets   = []string{"Acton", "Ada", "Addison", "Adeline", "Alcatraz", "Allston", "Ashby", "Bancroft", "Benvenue", "Berryman", "Blake", "Bonar", "Bonita", "Bowditch", "Buena", "California", "Camelia", "Carleton", "Carlotta", "Cedar", "Center", "Channing", "Chestnut", "Claremont", "Codornices", "College", "Cragmont", "Delaware", "Derby", "Dwight", "Eastshore", "Edith", "Elmwood", "Euclid", "Francisco", "Fresno", "Gilman", "Grizzly", "Harrison", "Hearst", "Heinz", "Henry", "Hillegass", "Holly", "Hopkins", "Josephine", "Kains", "King", "Le", "Conte", "Mabel", "Marin", "Martin", "Luther", "King", "MLK", "Milvia", "Monterey", "Napa", "Neilson", "Oregon", "Parker", "Piedmont", "Posen", "Rose", "Russell", "Sacramento", "Santa", "Fe", "Shattuck", "Solano", "Sonoma", "Spruce", "Telegraph", "The", "Alameda", "Thousand", "Oaks", "University", "Vine", "Virginia", "Ward", "Woolsey"}
	modifiers = []string{"street", "boulevard", "road", "path", "way", "avenue"}
	terms     = []string{"bike", "bicycle", "pedestrian", "vehicle", "injury", "victim", "versus", "transport", "concious", "breathing"}
)

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

type Config struct {
	openaiClient *audio.Client
	uploader     *s3manager.Uploader
	slackClient  *slack.Client
	webhookUrl   string
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"

	}

	// slack setup
	webhookUrl := os.Getenv("SLACK_WEBHOOK_URL")
	slackapiSecret := os.Getenv("SLACK_API_SECRET")
	var api *slack.Client
	if slackapiSecret == "" || webhookUrl == "" {
		log.Println("Missing SLACK_API_SECRET or SLACK_WEBHOOK_URL. Slack notifications disabled.")
	} else {
		api = slack.New(slackapiSecret)
	}

	// openai setup
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		log.Fatalf("Missing OPENAI_API_KEY")
	}

	s := openai.NewSession(os.Getenv("OPENAI_API_KEY"))
	openaiCli := audio.NewClient(s, "")

	// s3 setup
	s3Config := &aws.Config{
		Region:      aws.String("us-west-2"),
		Credentials: credentials.NewEnvCredentials(),
	}
	s3Session := session.New(s3Config)

	uploader := s3manager.NewUploader(s3Session)

	config := &Config{
		openaiClient: openaiCli,
		uploader:     uploader,
		slackClient:  api,
		webhookUrl:   webhookUrl,
	}

	mux := NewMux(config)

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// NewMux creates a new ServeMux router
func NewMux(config *Config) *http.ServeMux {
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

		result, err := url.JoinPath("https://dxjjyw8z8j16s.cloudfront.net", link[0])
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
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20+512)
		msg, err := handleTranscription(r.Context(), config, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		http.ServeContent(w, r, "", time.Now(), strings.NewReader(msg))
	})

	return mux
}

func handleTranscription(ctx context.Context, config *Config, r *http.Request) (string, error) {

	err := r.ParseMultipartForm(1 << 20)
	if err != nil {
		return "", err
	}
	js, _, err := r.FormFile("call_json")
	if err != nil {
		return "", err
	}
	defer js.Close()
	callJson, _ := ioutil.ReadAll(js)

	file, header, err := r.FormFile("call_audio")
	if err != nil {
		return "", err
	}
	defer file.Close()

	var meta Metadata
	err = json.Unmarshal(callJson, &meta)
	if err != nil {
		return "", err
	}
	filename := filepath.Base(header.Filename)
	filepath := fmt.Sprintf("%s/%d/%s", meta.ShortName, meta.Talkgroup, filename)
	msg, err := transcribeAndUpload(r.Context(), config, filepath, file, meta)
	if err != nil {
		return "", err
	}
	return msg, nil
}

// transcribeAndUpload transcribes the audio to text, posts the text to slack and persists the audio file to S3,
func transcribeAndUpload(ctx context.Context, config *Config, key string, reader io.Reader, metadata Metadata) (string, error) {

	data, _ := ioutil.ReadAll(reader)

	msg, err := whisper(ctx, config.openaiClient, bytes.NewReader(data))
	if err == nil {
		fmt.Println(key+": ", msg)
	}

	metadata.AudioText = msg
	metadata.URL = fmt.Sprintf("https://trunk-transcribe.fly.dev/audio?link=%s", key)

	wg, gctx := errgroup.WithContext(ctx)
	wg.Go(func() error {
		return uploadS3(gctx, config.uploader, key, bytes.NewReader(data), metadata)
	})
	wg.Go(func() error {
		return postToSlack(gctx, config, metadata)
	})
	err = wg.Wait()
	return msg, err
}

// whisper transcribes the audio with openai Whisper
func whisper(ctx context.Context, client *audio.Client, reader io.Reader) (string, error) {
	prompt := strings.Join(append(streets, append(modifiers, terms...)...), ", ")
	resp, err := client.CreateTranscription(ctx, &audio.CreateTranscriptionParams{
		Prompt:      prompt,
		Language:    "en",
		Audio:       reader,
		AudioFormat: "wav",
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// transcribeAndUpload uploads the audio to S3
func uploadS3(ctx context.Context, uploader *s3manager.Uploader, key string, reader io.Reader, meta Metadata) error {

	b, _ := json.Marshal(meta)

	s3Meta := make(map[string]*string)
	s3Meta["audio-text"] = aws.String(meta.AudioText)
	s3Meta["short-name"] = aws.String(meta.ShortName)
	s3Meta["call-length"] = aws.String(strconv.FormatInt(meta.CallLength, 10))
	s3Meta["talk-group"] = aws.String(strconv.FormatInt(meta.Talkgroup, 10))
	s3Meta["priority"] = aws.String(strconv.FormatInt(meta.Priority, 10))
	s3Meta["meta"] = aws.String(string(b))

	input := &s3manager.UploadInput{
		Bucket:      aws.String("scanner-berkeley"),         // bucket's name
		Key:         aws.String(key),                        // files destination location
		Body:        reader,                                 // content of the file
		Metadata:    s3Meta,                                 // metadata
		ContentType: aws.String("application/octet-stream"), // content type
	}
	_, err := uploader.UploadWithContext(ctx, input)
	fmt.Println("done uploading")
	return err
}

func postToSlack(ctx context.Context, config *Config, meta Metadata) error {
	if config.slackClient == nil {
		log.Println("Missing SLACK_API_SECRET or SLACK_WEBHOOK_URL. Slack notifications disabled.")
		return nil
	}
	if meta.AudioText == "" {
		log.Println("Empty audio text. Eliding slack post")
		return nil
	}

	blocks := strings.Split(meta.AudioText, ".")

	for i, block := range blocks {
		if i >= len(meta.SrcList) {
			break
		}
		src := meta.SrcList[i]
		tag := src.Tag
		if tag == "" {
			tag = strconv.FormatInt(src.Src, 10)
		}
		blocks[i] = tag + ": " + block
	}

	blocks = append(blocks, fmt.Sprintf("<%s|Audio>", meta.URL))
	sentances := strings.Join(blocks, "\n")

	attachment := slack.Attachment{
		Color:         "good",
		Fallback:      meta.AudioText,
		AuthorName:    meta.TalkgroupTag,
		AuthorSubname: meta.TalkGroupDesc,
		AuthorLink:    "https://github.com/radical-bike-lobby",
		AuthorIcon:    "https://avatars.githubusercontent.com/u/153021490",
		Text:          sentances,
		Footer:        fmt.Sprintf("%d seconds", meta.CallLength),
		Ts:            json.Number(strconv.FormatInt(time.Now().Unix(), 10)),
	}
	msg := slack.WebhookMessage{
		Attachments: []slack.Attachment{attachment},
	}

	err := slack.PostWebhookContext(ctx, config.webhookUrl, &msg)
	return err
}

func writeErr(w http.ResponseWriter, err error) {
	fmt.Println("Error: ", err.Error())
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// Structures to parse json of the form:
//
//	{
//	  "freq": 772693750,
//	  "start_time": 1702513859,
//	  "stop_time": 1702513867,
//	  "emergency": 0,
//	  "priority": 4,
//	  "mode": 0,
//	  "duplex": 0,
//	  "encrypted": 0,
//	  "call_length": 6,
//	  "talkgroup": 3105,
//	  "talkgroup_tag": "Berkeley PD1",
//	  "talkgroup_description": "Police Dispatch",
//	  "talkgroup_group_tag": "Law Dispatch",
//	  "talkgroup_group": "Berkeley",
//	  "audio_type": "digital",
//	  "short_name": "Berkeley",
//	  "freqList": [
//	    {
//	      "freq": 772693750,
//	      "time": 1702513859,
//	      "pos": 0,
//	      "len": 1.08,
//	      "error_count": "100",
//	      "spike_count": "7"
//	    },
//	    {
//	      "freq": 772693750,
//	      "time": 1702513862,
//	      "pos": 1.08,
//	      "len": 3.78,
//	      "error_count": "21",
//	      "spike_count": "1"
//	    },
//	  ],
//	  "srcList": [
//	    {
//	      "src": 3113003,
//	      "time": 1702513859,
//	      "pos": 0,
//	      "emergency": 0,
//	      "signal_system": "",
//	      "tag": "Dispatch"
//	    },
//	    {
//	      "src": 3124119,
//	      "time": 1702513862,
//	      "pos": 1.08,
//	      "emergency": 0,
//	      "signal_system": "",
//	      "tag": ""
//	    },
//	  ]
//	}
type Frequency struct {
	Freq int64   `json:"freq,omitempty"`
	Time int64   `json:"time,omitempty"`
	Pos  float64 `json:"pos,omitempty"`
}
type Source struct {
	Src          int64   `json:"src,omitempty"`
	Time         int64   `json:"time,omitempty"`
	Pos          float64 `json:"pos,omitempty"`
	Emergency    int64   `json:"emergency,omitempty"`
	SignalSystem string  `json:"signal_system,omitempty"`
	Tag          string  `json:"tag,omitempty"`
}

type Metadata struct {
	Freq              int64       `json:"freq,omitempty"`
	StartTime         int64       `json:"start_time,omitempty"`
	StopTime          int64       `json:"stop_time,omitempty"`
	Emergency         int64       `json:"emergency,omitempty"`
	Priority          int64       `json:"priority,omitempty"`
	Mode              int64       `json:"mode,omitempty"`
	Duplex            int64       `json:"duplex,omitempty"`
	Encrypted         int64       `json:"encrypted,omitempty"`
	CallLength        int64       `json:"call_length,omitempty"`
	Talkgroup         int64       `json:"talkgroup,omitempty"`
	TalkgroupTag      string      `json:"talkgroup_tag,omitempty"`
	TalkGroupDesc     string      `json:"talkgroup_description,omitempty"`
	TalkGroupGroupTag string      `json:"talkgroup_group_tag,omitempty"`
	TalkGroupGroup    string      `json:"talkgroup_group,omitempty"`
	AudioType         string      `json:"audio_type,omitempty"`
	ShortName         string      `json:"short_name,omitempty"`
	AudioText         string      `json:"audio_text,omitempty"`
	URL               string      `json:"url,omitempty"`
	SrcList           []Source    `json:"srcList,omitempty"`
	FreqList          []Frequency `json:"freqList,omitempty"`
}
