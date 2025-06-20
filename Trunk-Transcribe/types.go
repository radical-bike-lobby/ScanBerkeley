package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type SlackChannelID string

const (
	UCPD       SlackChannelID = "C06J8T3EUP9"
	BERKELEY   SlackChannelID = "C06A28PMXFZ"
	OAKLAND                   = "C070R7LGVDY"
	ALBANY                    = "C0713T4KMMX"
	EMERYVILLE                = "C07123TKG3E"
)

type TalkGroupID int64

const (
	ALTA_BATES_HOSPITAL = 5506
	CHILDRENS_HOSPITAL  = 5507
	HIGHLAND_HOSPITAL   = 5509
)

type SlackUserID string

const (
	EMILIE  SlackUserID = "U06H9NA2L4V"
	MARC                = "U03FTUS9SSD"
	NAVEEN              = "U0531U1RY1W"
	JOSE                = "U073Q372CP9"
	STEPHAN             = "U06UWE5EDAT"
	HELEN               = "U08155VNVRQ"
)

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
	Segments          []string    `json:"segments,omitempty"`
}

type Notifs struct {
	Include    []string
	Regex      *regexp.Regexp
	NotRegex   *regexp.Regexp
	Channels   []SlackChannelID //the channels to listen on
	TalkGroups []TalkGroupID    //individual talkgroups to listen to (could be exclusive of channels)
}

func (n Notifs) MatchesText(channelID SlackChannelID, talkgroupID TalkGroupID, text string, words []string) bool {
	listeningToChannel := slices.Contains(n.Channels, channelID)
	listeningToTalkgroup := slices.Contains(n.TalkGroups, TalkGroupID(talkgroupID))

	switch {
	case !listeningToChannel && !listeningToTalkgroup: // user not listening to channel or talkgroup
		return false
	case n.NotRegex != nil && n.NotRegex.MatchString(text): // notregex matches text. skip
		return false
	case n.Regex != nil && n.Regex.MatchString(text): // regex matches text. append
		return true
	}

	// check the included keywords

	for _, keyword := range n.Include {
		keyword = strings.ToLower(keyword)
		sequence := wordsRegex.FindAllString(keyword, -1)
		for chunk := range slices.Chunk(words, len(sequence)) {
			if slices.Equal(chunk, sequence) {
				return true
			}
		}
	}
	return false
}

type SlackMeta struct {
	Mentions []string `json:"mentions,omitempty"`
	Address  Address  `json:"address,omitempty"`
}

// Address struct encapsulates address info to extract from transcription text
// Example transcriptions: "2605 Durant", "Can you start for Russell and California please?"
type Address struct {
	City           string   `json:"city,omitempty"`
	PrimaryAddress string   `json:"primary,omitempty"`
	Streets        []string `json:"streets,omitempty"`
}

func (addr Address) AppendStreet(street string) Address {
	for _, st := range addr.Streets {
		if st == street {
			return addr
		}
	}
	addr.Streets = append(addr.Streets, street)
	return addr
}

func (addr Address) String() string {
	switch {
	case addr.PrimaryAddress != "":
		return addr.PrimaryAddress
	case len(addr.Streets) > 1:
		return addr.Streets[0] + " and " + addr.Streets[1]
	default:
		return ""
	}
}

type Segment struct {
	Start             float32 `json:"start,omitempty"`
	End               float32 `json:"end,omitempty"`
	Text              string  `json:"text,omitempty"`
	AvgLogProb        float64 `json:"avg_logprob,omitempty"`
	NoSpeechProb      float64 `json:"no_speech_prob,omitempty"`
	CompressionRation float64 `json:"compression_ratio,omitempty"`
}

type TranscriptionInfo struct {
	Language  string    `json:"language,omitempty"`
	Duration  string    `json:"duration,omitempty"`
	Text      string    `json:"text,omitempty"`
	WordCount int       `json:"word_count,omitempty"`
	Segments  []Segment `json:"segments,omitempty"`
}

type CloudflareWhisperInput struct {
	Audio  string `json:"audio,omitempty"`
	Prompt string `json:"initial_prompt,omitempty"`
	Prefix string `json:"prefix,omitempty"`
}

type CloudflareWhisperOutput struct {
	Result   TranscriptionInfo `json:"result,omitempty"`
	Success  bool              `json:"success,omitempty"`
	Errors   interface{}       `json:"errors,omitempty"`
	Messages []string          `json:"messages,omitempty"`
}

func NewTranscriptionRequest(name string, data, meta []byte) (*TranscriptionRequest, error) {
	var metadata Metadata
	err := json.Unmarshal(meta, &metadata)
	if err != nil {
		return nil, err
	}
	return &TranscriptionRequest{
		Filename: name,
		Data:     data,
		Meta:     metadata,
		MetaRaw:  meta,
	}, nil
}

type TranscriptionRequest struct {
	Filename string
	Data     []byte
	Meta     Metadata
	MetaRaw  []byte
}

func (t *TranscriptionRequest) FilePath() string {
	return fmt.Sprintf("%s/%d/%s", t.Meta.ShortName, t.Meta.Talkgroup, t.Filename)
}
