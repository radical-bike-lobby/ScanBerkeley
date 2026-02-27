package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalMetadata(t *testing.T) {
	jsonData, err := os.ReadFile("testdata/metadata.json")
	require.NoError(t, err, "should be able to read test data file")

	var meta Metadata
	err = json.Unmarshal(jsonData, &meta)
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

func TestExtractSlackMeta(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected SlackMeta
	}{
		{
			name: "empty text",
			text: "",
			expected: SlackMeta{
				Mentions: []string{},
				Address:  Address{},
			},
		},
		{
			name: "single street address",
			text: "incident at Bancroft and Telegraph",
			expected: SlackMeta{
				Address: Address{
					Streets: []string{"Bancroft", "Telegraph"},
				},
			},
		},
		{
			name: "address with number",
			text: "incident at 2500 Bancroft",
			expected: SlackMeta{
				Address: Address{
					PrimaryAddress: "2500 Bancroft",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := createTestMetadata()
			meta.AudioText = tt.text

			result := ExtractSlackMeta(meta, BERKELEY, notifsMap)

			assert.Equal(t, tt.expected.Address.Streets, result.Address.Streets, "streets should match")
			assert.Equal(t, tt.expected.Address.PrimaryAddress, result.Address.PrimaryAddress, "primary address should match")
		})
	}
}
