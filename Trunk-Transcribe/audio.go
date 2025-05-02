package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	ffmpeg "github.com/u2takey/ffmpeg-go"
	"google.golang.org/api/option"
)

const (
	deepFilterCmd      = "./deep-filter"
	silenceThreshold   = 100                    // Adjust as needed
	minSilenceDuration = 200 * time.Millisecond // Minimum duration of silence to remove
)

// google gemini
var geminiApiKey string = os.Getenv("GEMINI_API_KEY")

// cloudflare setup
var cloudflareApiToken string = os.Getenv("CLOUDFLARE_API_TOKEN")
var cloudflareAccountID string = os.Getenv("CLOUDFLARE_ACCOUNT_ID")
var cloudflareWhisperUrl string = "https://api.cloudflare.com/client/v4/accounts/" + cloudflareAccountID + "/ai/run/@cf/openai/whisper-large-v3-turbo"

// Utility audio functions including silence removal, enhancement transcription

// whisper transcribes the audio with cloudflare Whisper
func whisper(ctx context.Context, data []byte) (msg string, segments []string, err error) {
	prompt := strings.Join(append(streets, append(modifiers, terms...)...), ", ")

	enc := base64.StdEncoding.EncodeToString(data)
	payload, err := json.Marshal(CloudflareWhisperInput{
		Audio:  enc,
		Prompt: prompt,
	})

	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", cloudflareWhisperUrl, bytes.NewReader(payload))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cloudflareApiToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error calling cloudflare: %v\n", err)
		return "", nil, err
	}
	defer resp.Body.Close()
	var output CloudflareWhisperOutput
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	err = json.Unmarshal(body, &output)
	if err != nil {
		return "", nil, err
	}

	msg = output.Result.Text
	//fmt.Println("Response from cloudflare: ", string(body))
	for _, segment := range output.Result.Segments {
		segments = append(segments, segment.Text)
	}

	return msg, segments, nil
}

func gemini(ctx context.Context, data []byte) (string, error) {

	client, err := genai.NewClient(ctx, option.WithAPIKey(geminiApiKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	prompt := strings.Join(append(streets, append(modifiers, terms...)...), ", ")
	parts := []genai.Part{
		genai.Blob{MIMEType: "audio/mp3", Data: data},
		genai.Text("Please transcribe the audio. "),
		genai.Text("Ignore silences."),
		genai.Text("Here are some correction terms: " + prompt),
	}

	model := client.GenerativeModel("gemini-1.5-pro")
	resp, err := model.GenerateContent(ctx, parts...)
	if err != nil {
		return "", err
	}

	var transcriptionParts []string
	for _, c := range resp.Candidates {
		if c.Content == nil {
			continue
		}
		for _, part := range c.Content.Parts {
			line := fmt.Sprintf("%v", part)
			if strings.TrimSpace(line) == "" {
				continue
			}
			transcriptionParts = append(transcriptionParts, line)
		}
	}

	msg := strings.Join(transcriptionParts, "\n")
	return msg, nil
}

// deepFilter runs an audio enhancement framework on the data based on the Deepfilter ai model
func deepFilter(ctx context.Context, data []byte) ([]byte, error) {

	dir := os.TempDir()
	audioFile, err := os.CreateTemp(dir, "")
	if err != nil {
		return nil, err
	}
	defer os.Remove(audioFile.Name())

	err = os.WriteFile(audioFile.Name(), data, 0666)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, deepFilterCmd, "--pf", "-v", "-o", dir, audioFile.Name())
	err = cmd.Run()
	if err != nil {
		return nil, err
	}

	return io.ReadAll(audioFile)
}

func removeSilence(ctx context.Context, data []byte) io.Reader {
	reader, writer := io.Pipe()

	go func() {
		stream := ffmpeg.Input("pipe:")

		stream.Context = ctx

		err := stream.
			WithInput(bytes.NewBuffer(data)).
			Output("pipe:", ffmpeg.KwArgs{
				"af":          "silenceremove=1:0:-50dB",
				"format":      "wav",
				"hide_banner": "",
				"loglevel":    "error",
			}).
			WithOutput(writer).
			Silent(true).
			ErrorToStdOut().
			Run()

		switch err {
		case nil:
			writer.Close()
		default:
			writer.CloseWithError(err)
		}
	}()

	return reader
}
