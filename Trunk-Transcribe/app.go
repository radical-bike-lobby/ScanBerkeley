package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

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

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
		}

		t.ExecuteTemplate(w, "index.html.tmpl", data)
	})

	http.HandleFunc("/transcribe", func(w http.ResponseWriter, r *http.Request) {

		filepath := r.URL.Query().Get("filepath")
		if filepath == "" {
			writeErr(w, errors.New("missing filepath query param"))
			return
		}

		reader, err := r.MultipartReader()
		if err != nil {
			writeErr(w, err)
			return
		}
		for {
			p, err := reader.NextPart()
			switch err {
			case io.EOF:
				return
			case nil:
			default:
				writeErr(w, err)
				return
			}

			fmt.Println("processing form value: " + p.FormName())
			switch strings.ToLower(p.FormName()) {
			case "call_json":
				// TODO: write metadata to sqlite
			case "call_audio":
				if err = transcribeAndUpload(r.Context(), client, uploader, filepath, p); err != nil {
					writeErr(w, err)
					return
				}
			}
		}
	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func transcribeAndUpload(ctx context.Context, client *audio.Client, uploader *s3manager.Uploader, key string, reader io.Reader) error {
	read, write := io.Pipe()
	defer write.Close()

	teeReader := io.TeeReader(reader, write)

	var wg sync.WaitGroup
	defer wg.Wait()

	go func() {
		wg.Add(1)
		defer wg.Done()
		msg, err := transcribe(ctx, client, read)
		if err != nil {
			fmt.Println("Error invoking transcribe: ", err.Error())
		} else {
			fmt.Println("transcription success: ", msg)
		}
	}()
	return uploadS3(ctx, uploader, key, teeReader)
}

func transcribe(ctx context.Context, client *audio.Client, reader io.Reader) (string, error) {
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

func uploadS3(ctx context.Context, uploader *s3manager.Uploader, key string, reader io.Reader) error {
	input := &s3manager.UploadInput{
		Bucket:      aws.String("scanner-berkeley"),         // bucket's name
		Key:         aws.String(key),                        // files destination location
		Body:        reader,                                 // content of the file
		ContentType: aws.String("application/octet-stream"), // content type
	}
	_, err := uploader.UploadWithContext(ctx, input)
	return err
}

func writeErr(w http.ResponseWriter, err error) {
	fmt.Println("Error: ", err.Error())
	http.Error(w, err.Error(), http.StatusInternalServerError)
}