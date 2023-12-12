package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/rakyll/openai-go"
	"github.com/rakyll/openai-go/audio"
)

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

type Config struct {
	client   *audio.Client
	uploader *s3manager.Uploader
}

type Metadata struct {
	Freq              int64  `json:"freq:omitempty"`
	StartTime         int64  `json:"start_time,omitempty"`
	StopTime          int64  `json:"stop_time,omitempty"`
	Emergency         int64  `json:"emergency,omitempty"`
	Priority          int64  `json:"priority,omitempty"`
	Mode              int64  `json:"mode,omitempty"`
	Duplex            int64  `json:"duplex,omitempty"`
	Encrypted         int64  `json:"encrypted,omitempty"`
	CallLength        int64  `json:"call_length,omitempty"`
	Talkgroup         int64  `json:"talkgroup,omitempty"`
	TalkgroupTag      string `json:"talkgroup_tag,omitempty"`
	TalkGroupDesc     string `json:"talkgroup_description,omitempty"`
	TalkGroupGroupTag string `json:"talkgroup_group_tag,omitempty"`
	TalkGroupGroup    string `json:"talkgroup_group,omitempty"`
	AudioType         string `json:"audio_type,omitempty"`
	ShortName         string `json:"short_name,omitempty"`
	AudioText         string `json:"audio_text,omitempty"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"

	}

	s := openai.NewSession(os.Getenv("OPENAI_API_KEY"))
	client := audio.NewClient(s, "")

	s3Config := &aws.Config{
		Region:      aws.String("us-west-2"),
		Credentials: credentials.NewEnvCredentials(),
	}
	s3Session := session.New(s3Config)

	uploader := s3manager.NewUploader(s3Session)

	config := &Config{
		client:   client,
		uploader: uploader,
	}

	// Homepage
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
		}
		t.ExecuteTemplate(w, "index.html.tmpl", data)
	})

	// transribe endpoint
	http.HandleFunc("/transcribe", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20+512)
		msg, err := handleTranscription(r.Context(), config, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		http.ServeContent(w, r, "", time.Now(), strings.NewReader(msg))
	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
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

// transcribeAndUpload persists the audio file to S3 and transcribes it with openai Whisper
func transcribeAndUpload(ctx context.Context, config *Config, key string, reader io.Reader, metadata Metadata) (string, error) {

	data, _ := ioutil.ReadAll(reader)

	msg, err := whisper(ctx, config.client, bytes.NewReader(data))
	if err == nil {
		fmt.Println(key+": ", msg)
	}

	metadata.AudioText = msg
	uploadS3(ctx, config.uploader, key, bytes.NewReader(data), metadata)

	return msg, err
}

// transcribeAndUpload transcribes the audio with openai Whisper
func whisper(ctx context.Context, client *audio.Client, reader io.Reader) (string, error) {
	resp, err := client.CreateTranscription(ctx, &audio.CreateTranscriptionParams{
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

	s3Meta := make(map[string]*string)
	s3Meta["audio-text"] = aws.String(meta.AudioText)
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
	fmt.Println("done uploading")
	return err
}

func writeErr(w http.ResponseWriter, err error) {
	fmt.Println("Error: ", err.Error())
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
