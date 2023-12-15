package main

import (
    "encoding/json"
    "testing"

    "github.com/stretchr/testify/assert"
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
