package main

import (
    "encoding/json"
    "io/ioutil"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

var data = `{
"freq": 772393750,
"start_time": 1702617247,
"stop_time": 1702617252,
"emergency": 0,
"priority": 4,
"mode": 0,
"duplex": 0,
"encrypted": 0,
"call_length": 4,
"talkgroup": 3105,
"talkgroup_tag": "Berkeley PD1",
"talkgroup_description": "Police Dispatch",
"talkgroup_group_tag": "Law Dispatch",
"talkgroup_group": "Berkeley",
"audio_type": "digital",
"short_name": "Berkeley",
"freqList": [ {"freq": 772393750, "time": 1702617247, "pos": 0.00, "len": 2.88, "error_count": "0", "spike_count": "0"}, {"freq": 772393750, "time": 1702617251, "pos": 2.88, "len": 1.44, "error_count": "46", "spike_count": "3"} ],
"srcList": [ {"src": 3124119, "time": 1702617247, "pos": 0.00, "emergency": 0, "signal_system": "", "tag": ""}, {"src": 3113008, "time": 1702617251, "pos": 2.88, "emergency": 0, "signal_system": "", "tag": "Dispatch"} ]
}`

var filename = "Berkeley/2105/2105-1702705979_772093750.1-call_130267.wav"

func TestUnmarshalMetadata(t *testing.T) {
    var meta Metadata
    err := json.Unmarshal([]byte(data), &meta)
    assert.NoError(t, err)

    assert.NotNil(t, meta, "meta must not nil")
    assert.NotEmpty(t, meta.SrcList, "source list must not be nil")
    assert.Equal(t, meta.Freq, int64(772393750), "freq match")
    assert.Equal(t, meta.TalkgroupTag, "Berkeley PD1", "talkgrouptag match")
    assert.Equal(t, meta.TalkGroupDesc, "Police Dispatch", "talkgroupdesc match")
    assert.Equal(t, meta.SrcList[0].Src, int64(3124119), "first source must be correct")
    assert.Equal(t, meta.SrcList[1].Src, int64(3113008), "second source must be correct")
    assert.Equal(t, meta.SrcList[1].Tag, "Dispatch", "second source must be Dispatch")
}

func TestAudioPlayer(t *testing.T) {
    mux := NewMux(nil)

    rr := httptest.NewRecorder()
    req, err := http.NewRequest("GET", "/audio?link="+filename, nil)
    require.NoError(t, err)

    mux.ServeHTTP(rr, req)
    assert.Equal(t, 200, rr.Code)

    b, err := ioutil.ReadAll(rr.Body)
    require.NoError(t, err)

    expect := "https://dxjjyw8z8j16s.cloudfront.net/" + filename

    assert.Contains(t, string(b), expect, string(b)+"\n\nshould contain:\n\n"+expect)
}

func TestSplitSentance(t *testing.T) {
    tests := []struct {
        name     string
        sentance string
        expect   []string
    }{
        {
            name:     "question mark",
            sentance: "114 Control, do you have traffic? Affirm, we have a car on our way",
            expect: []string{
                "114 Control, do you have traffic",
                "Affirm, we have a car on our way",
            },
        },
        {
            name:     "commas",
            sentance: "Copy, just confirming southeast corner, Alcatraz and Adelaide. Confirming, it's in the parking lot, it's on the sidewalk, it's out of the roadway",
            expect: []string{
                "Copy, just confirming southeast corner, Alcatraz and Adelaide",
                "Confirming, it's in the parking lot, it's on the sidewalk, it's out of the roadway",
            },
        },
    }

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            blocks := puncRegex.Split(test.sentance, -1)
            assert.Equal(t, test.expect, blocks)
        })
    }
}

func TestMentions(t *testing.T) {
    tests := []struct {
        name     string
        sentance string
        expect   []string
    }{
        {
            name:     "single",
            sentance: "Can you tell me one more time? you got me en route to 1071 in the SRT van? In the SRT van",
            expect:   []string{"<@U06H9NA2L4V>"},
        },
        {
            name:     "multi word match",
            sentance: "Fancroft and Piedmont,we've got a vehicle versus bike, and we've got an involved party on the phone,we've got BFD and RUN as well",
            expect:   []string{"<@U06H9NA2L4V>", "<@U0531U1RY1W>", "<@U03FTUS9SSD>"},
        },
        {
            name:     "negate",
            sentance: "Fancroft and Piedmont, no weapon seen",
            expect:   nil,
        },
        {
            name:     "negate plural",
            sentance: "Fancroft and Piedmont, no weapons seen",
            expect:   nil,
        },
        {
            name:     "weapon",
            sentance: "Fancroft and Piedmont, weapon seen",
            expect:   []string{"<@U06H9NA2L4V>"},
        },
        {
            name:     "plural",
            sentance: "Fancroft and Piedmont, weapons seen",
            expect:   []string{"<@U06H9NA2L4V>"},
        },
        {
            name:     "Capitalized",
            sentance: "ROSE",
            expect:   []string{"<@U0531U1RY1W>"},
        },
        {
            name:     "substring",
            sentance: " It's going to be a good 242 with the prosecution requested, BFD declined, and clear for a suspect description.",
            expect:   nil,
        },
        {
            name:     "rose",
            sentance: "It's going to be a good 242 with the prosecution requested, BFD declined, and clear for a suspect description. Rose and Shattuck.",
            expect:   []string{"<@U0531U1RY1W>"},
        },
        {
            name:     "hyphen",
            sentance: "Can you mark a 10-15 time? Copy, 16-05",
            expect:   []string{"<@U06H9NA2L4V>"},
        },
    }

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            meta := ExtractSlackMeta(Metadata{AudioText: test.sentance}, notifsMap)
            assert.ElementsMatch(t, test.expect, meta.Mentions)
        })
    }
}

func TestStreets(t *testing.T) {
    tests := []struct {
        name     string
        sentance string
        expect   SlackMeta
    }{
        {
            name:     "none",
            sentance: "Can you tell me one more time? you got me en route to 1071 in the SRT van",
            expect:   SlackMeta{},
        },
        {
            name:     "single",
            sentance: "Can you tell me one more time? you got me en route to 1071 in the SRT van? In the SRT van at Bancroft and Channing",
            expect: SlackMeta{
                Address: Address{Streets: []string{"Bancroft", "Channing"}},
            },
        },
        {
            name:     "cross streets",
            sentance: "Can you tell me one more time? you got me en route to 1071 in the SRT van? In the SRT van at Bancroft and Channing",
            expect: SlackMeta{
                Address: Address{Streets: []string{"Bancroft", "Channing"}},
            },
        },
        {

            name:     "three",
            sentance: "Can you tell me one more time? you got me en route to 1071 in the SRT van? In the SRT van at Bancroft between Channing and Milvia",
            expect: SlackMeta{
                Address: Address{Streets: []string{"Bancroft", "Channing", "Milvia"}},
            },
        },
        {

            name:     "duplicates",
            sentance: "Can you tell me one more time? you got me en route to 1071 in the SRT van? In the SRT van at Bancroft between Channing and Milvia, repeat Channing and Milvia",
            expect: SlackMeta{
                Address: Address{Streets: []string{"Bancroft", "Channing", "Milvia"}},
            },
        },
        {
            name:     "address number",
            sentance: "Can you tell me one more time? you got me en route to 1071 in the SRT van? In the SRT van at 3049 Bancroft",
            expect: SlackMeta{
                Address: Address{
                    Streets:        []string{},
                    PrimaryAddress: "3049 Bancroft",
                },
            },
        },
        {
            name:     "address number two streets",
            sentance: "Can you tell me one more time? you got me en route to 1071 in the SRT van? In the SRT van at 3049 Bancroft and Channing",
            expect: SlackMeta{
                Address: Address{
                    Streets:        []string{"Channing"},
                    PrimaryAddress: "3049 Bancroft",
                },
            },
        },
    }

    for _, test := range tests {
        t.Run(test.name, func(t *testing.T) {
            meta := ExtractSlackMeta(Metadata{AudioText: test.sentance}, notifsMap)
            assert.ElementsMatch(t, test.expect.Address.Streets, meta.Address.Streets)

            assert.Equal(t, test.expect.Address.String(), meta.Address.String())
            if test.expect.Address.PrimaryAddress != "" || len(test.expect.Address.Streets) > 0 {
                assert.NotEmpty(t, meta.Address.String())
            }
        })
    }
}

// func TestTrunkTranscribe(t *testing.T) {
//     mux := NewMux(nil)

//     rr := httptest.NewRecorder()
//     req, err := http.NewRequest("POST", "/transcribe", nil)
//     require.NoError(t, err)

//     mux.ServeHTTP(rr, req)
//     b, _ := ioutil.ReadAll(rr.Body)
//     assert.Equal(t, "request Content-Type isn't multipart/form-data\n", string(b))

//     var buf bytes.Buffer
//     w := multipart.NewWriter(&buf)

//     err = w.WriteField("call_json", data)
//     require.NoError(t, err)

//     writer, err := w.CreateFormFile("call_audio", filename)
//     require.NoError(t, err)
//     _, err = writer.Write([]byte("some audio binary bits"))
//     require.NoError(t, err)
//     w.Close()
//     req, err = http.NewRequest("POST", "/transcribe", &buf)
//     require.NoError(t, err)
//     req.Header.Set("Content-Type", w.FormDataContentType())

//     mux.ServeHTTP(rr, req)

//     assert.Equal(t, 200, rr.Code, "recevieved unexpected response: "+string(b))
// }
