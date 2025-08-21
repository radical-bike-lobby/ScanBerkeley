package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
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
	TAJ                 = "U08V90KL9SS"
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
		Filename:     name,
		Data:         data,
		Meta:         metadata,
		MetaRaw:      meta,
		PostToSlack:  true,
		UploadToRdio: true,
	}, nil
}

type TranscriptionRequest struct {
	Filename     string
	Data         []byte
	Meta         Metadata
	MetaRaw      []byte
	PostToSlack  bool
	UploadToRdio bool
}

func (t *TranscriptionRequest) FilePath() string {
	return fmt.Sprintf("%s/%d/%s", t.Meta.ShortName, t.Meta.Talkgroup, t.Filename)
}

type Call struct {
	Id             any       `json:"id"`
	Audio          []byte    `json:"audio"`
	AudioName      string    `json:"audioName"`
	AudioType      string    `json:"audioType"`
	DateTime       time.Time `json:"dateTime"`
	Frequencies    any       `json:"frequencies"`
	Frequency      int64     `json:"frequency"`
	Patches        any       `json:"patches"`
	Source         any       `json:"source"`
	Sources        any       `json:"sources"`
	System         uint      `json:"system"`
	Talkgroup      int64     `json:"talkgroup"`
	SystemLabel    any       `json:"systemLabel"`
	TalkgroupGroup string    `json:"talkgroupGroup"`
	TalkgroupLabel string    `json:"talkgroupLabel"`
	TalkgroupName  string    `json:"talkgroupName"`
	TalkgroupTag   string    `json:"talkgroupTag"`
	Units          any       `json:"units"`
}

type Unit struct {
	Id    uint   `json:"id"`
	Label string `json:"label"`
	Order uint   `json:"order"`
}

type Units struct {
	List  []*Unit
	mutex sync.Mutex
}

func NewUnits() *Units {
	return &Units{
		List:  []*Unit{},
		mutex: sync.Mutex{},
	}
}

func (units *Units) Add(id uint, label string) (*Units, bool) {
	added := true

	for _, u := range units.List {
		if u.Id == id {
			added = false
			break
		}
	}

	if added {
		units.List = append(units.List, &Unit{Id: id, Label: label})
	}

	return units, added
}

func (call *Call) IsValid() (ok bool, err error) {
	ok = true

	if len(call.Audio) <= 44 {
		ok = false
		err = errors.New("no audio")
	}

	if call.DateTime.Unix() == 0 {
		ok = false
		err = errors.New("no datetime")
	}

	if call.System < 1 {
		ok = false
		err = errors.New("no system")
	}

	if call.Talkgroup < 1 {
		ok = false
		err = errors.New("no talkgroup")
	}

	return ok, err
}

func (call *Call) ToMetadata() (Metadata, error) {

	metadata := Metadata{
		Freq:              call.Frequency,
		StartTime:         0,
		StopTime:          0,
		Emergency:         0,
		Priority:          0,
		Mode:              0,
		Duplex:            0,
		Encrypted:         0,
		CallLength:        0,
		Talkgroup:         call.Talkgroup,
		TalkgroupTag:      call.TalkgroupTag,
		TalkGroupDesc:     call.TalkgroupLabel,
		TalkGroupGroupTag: call.TalkgroupTag,
		TalkGroupGroup:    call.TalkgroupGroup,
		AudioType:         call.AudioType,
		ShortName:         call.TalkgroupName,
		AudioText:         "",
		URL:               "",
		FreqList:          nil,
		Segments:          nil,
	}

	metadata.SrcList = []Source{}
	b, err := json.Marshal(call.Sources)
	if err != nil {
		return metadata, err
	}
	err = json.Unmarshal(b, metadata.SrcList)
	if err != nil {
		return metadata, err
	}

	return metadata, nil
}

func (call *Call) ToJson() (string, error) {
	audio := call.Audio
	call.Audio = nil
	defer func() {
		call.Audio = audio
	}()

	if b, err := json.Marshal(call); err == nil {
		return string(b), nil
	} else {
		return "", fmt.Errorf("call.tojson: %v", err)
	}
}

func (call *Call) ParseMultipartContent(p *multipart.Part, b []byte) {
	switch p.FormName() {
	case "audio":
		call.Audio = b
		call.AudioName = p.FileName()

	case "audioName":
		call.AudioName = string(b)
		call.AudioType = mime.TypeByExtension(path.Ext(string(b)))

	case "dateTime":
		if regexp.MustCompile(`^[0-9]+$`).Match(b) {
			if i, err := strconv.Atoi(string(b)); err == nil {
				call.DateTime = time.Unix(int64(i), 0).UTC()
			}
		} else {
			call.DateTime, _ = time.Parse(time.RFC3339, string(b))
			call.DateTime = call.DateTime.UTC()
		}

	case "frequencies":
		var f any
		if err := json.Unmarshal(b, &f); err == nil {
			switch v := f.(type) {
			case []any:
				var frequencies = []map[string]any{}
				for _, f := range v {
					freq := map[string]any{}
					switch v := f.(type) {
					case map[string]any:
						switch v := v["errorCount"].(type) {
						case float64:
							if v >= 0 {
								freq["errorCount"] = uint(v)
							}
						}
						switch v := v["freq"].(type) {
						case float64:
							if v > 0 {
								freq["freq"] = uint(v)
							}
						}
						switch v := v["len"].(type) {
						case float64:
							if v >= 0 {
								freq["len"] = uint(v)
							}
						}
						switch v := v["pos"].(type) {
						case float64:
							if v >= 0 {
								freq["pos"] = uint(v)
							}
						}
						switch v := v["spikeCount"].(type) {
						case float64:
							if v >= 0 {
								freq["spikeCount"] = uint(v)
							}
						}
					}
					frequencies = append(frequencies, freq)
				}
				call.Frequencies = frequencies
			}
		}

	case "frequency":
		if i, err := strconv.Atoi(string(b)); err == nil && i > 0 {
			call.Frequency = int64(i)
		}

	case "patches", "patched_talkgroups":
		var (
			f       any
			patches = []uint{}
		)
		if err := json.Unmarshal(b, &f); err == nil {
			switch v := f.(type) {
			case []any:
				for _, patch := range v {
					switch v := patch.(type) {
					case float64:
						if v > 0 {
							patches = append(patches, uint(v))
						}
					}
				}
			}
			call.Patches = patches
		}

	case "source":
		if i, err := strconv.Atoi(string(b)); err == nil {
			call.Source = int(i)
		}

	case "sources":
		var (
			f     any
			units *Units
		)
		if err := json.Unmarshal(b, &f); err == nil {
			switch v := f.(type) {
			case []any:
				var sources = []map[string]any{}
				for _, f := range v {
					src := map[string]any{}
					switch v := f.(type) {
					case map[string]any:
						switch v := v["pos"].(type) {
						case float64:
							if v >= 0 {
								src["pos"] = uint(v)
							}
						}
						switch s := v["src"].(type) {
						case float64:
							if s > 0 {
								src["src"] = uint(s)
								switch t := v["tag"].(type) {
								case string:
									if units == nil {
										units = NewUnits()
									}
									switch units := call.Units.(type) {
									case *Units:
										units.Add(uint(s), t)
									}
								}
							}
						}
					}
					sources = append(sources, src)
				}
				call.Sources = sources
				call.Units = units
			}
		}

	case "system", "systemId":
		if i, err := strconv.Atoi(string(b)); err == nil && i > 0 {
			call.System = uint(i)
		}

	case "systemLabel":
		call.SystemLabel = string(b)

	case "talkgroup", "talkgroupId":
		if i, err := strconv.Atoi(string(b)); err == nil && i > 0 {
			call.Talkgroup = int64(i)
		}

	case "talkgroupGroup":
		if s := string(b); len(s) > 0 && s != "-" {
			call.TalkgroupGroup = s
		}

	case "talkgroupLabel":
		if s := string(b); len(s) > 0 && s != "-" {
			call.TalkgroupLabel = s
		}

	case "talkgroupName":
		if s := string(b); len(s) > 0 && s != "-" {
			call.TalkgroupName = s
		}

	case "talkgroupTag":
		if s := string(b); len(s) > 0 && s != "-" {
			call.TalkgroupTag = s
		}
	}
}
