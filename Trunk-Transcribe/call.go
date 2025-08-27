package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"path"
	"regexp"
	"strconv"
	"sync"
	"time"
)

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
	err = json.Unmarshal(b, &metadata.SrcList)
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
