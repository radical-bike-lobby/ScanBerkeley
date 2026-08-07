package main

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var filename = "Berkeley/2105/2105-1702705979_772093750.1-call_130267.wav"

func TestAudioPlayer(t *testing.T) {

	mux := mux(nil, nil)

	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/audio?link="+filename, nil)
	require.NoError(t, err)

	mux.ServeHTTP(rr, req)
	assert.Equal(t, 200, rr.Code)

	b, err := io.ReadAll(rr.Body)
	require.NoError(t, err)

	expect := "https://pub-85c4b9a9667540e99c0109c068c47e0f.r2.dev/" + filename

	assert.Contains(t, string(b), expect, string(b)+"\n\nshould contain:\n\n"+expect)
}

func TestSplitSentence(t *testing.T) {
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
			name:     "multi word regex match",
			sentance: "Fancroft and Piedmont,we've got a vehicle versus bike, and we've got an involved party on the phone,we've got BFD and RUN as well",
			expect:   []string{"<@U06H9NA2L4V>", "<@U0531U1RY1W>", "<@U03FTUS9SSD>"},
		},
		{
			name:     "multi word term match",
			sentance: "112, Tom, attach me to the 1033, Frank, and I'm 10-9-7.",
			expect:   []string{"<@U06H9NA2L4V>"},
		},
		{
			name:     "versus match - car vs ped",
			sentance: "Fancroft and Piedmont,we've got a car versus ped, and we've got an involved party on the phone,we've got BFD and RUN as well",
			expect:   []string{"<@U06H9NA2L4V>", "<@U0531U1RY1W>", "<@U03FTUS9SSD>"},
		},
		{
			name:     "versus match - bicycle vs ped",
			sentance: "Fancroft and Piedmont,we've got a bicycle versus ped, and we've got an involved party on the phone,we've got BFD and RUN as well",
			expect:   []string{"<@U06H9NA2L4V>", "<@U0531U1RY1W>", "<@U03FTUS9SSD>"},
		},
		{
			name:     "versus match - auto vs. ped",
			sentance: "Fancroft and Piedmont,we've got a auto vs. ped, and we've got an involved party on the phone,we've got BFD and RUN as well",
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
			sentance: "FANCROFT AND PIEDMONT, WEAPONS SEEN",
			expect:   []string{"<@U06H9NA2L4V>"},
		},
		{
			name:     "substring",
			sentance: " It's going to be a good 242 with the prosecution requested, BFD declined, and clear for a suspect description.",
			expect:   nil,
		},
		{
			name:     "hyphen",
			sentance: "Can you mark a 10-15 time? Copy, 16-05",
			expect:   []string{"<@U06H9NA2L4V>"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			meta := ExtractSlackMeta(Metadata{AudioText: test.sentance}, BERKELEY, notifsMap)
			for _, elem := range test.expect {
				assert.Contains(t, meta.Mentions, elem)
			}
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
			sentance: "Can you tell me one more time? you got me en route to 1071 in the SRT van? In the SRT van at 3049 Bancroft and Channing and Milvia",
			expect: SlackMeta{
				Address: Address{
					Streets:        []string{"Channing", "Milvia"},
					PrimaryAddress: "3049 Bancroft",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			meta := ExtractSlackMeta(Metadata{AudioText: test.sentance}, BERKELEY, notifsMap)
			assert.ElementsMatch(t, test.expect.Address.Streets, meta.Address.Streets)

			assert.Equal(t, test.expect.Address.String(), meta.Address.String())
			if test.expect.Address.PrimaryAddress != "" || len(test.expect.Address.Streets) > 0 {
				assert.NotEmpty(t, meta.Address.String())
			}
		})
	}
}

func TestSilenceRemove(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available, skipping silence removal test")
	}
	ctx := context.Background()

	silenceAudioData, err := os.ReadFile("testdata/silence_audio.txt")
	require.NoError(t, err, "should be able to read audio test data")

	data, err := base64.StdEncoding.DecodeString(string(silenceAudioData))
	require.NoError(t, err, "should be able to decode base64 audio data")

	reader := removeSilence(ctx, data)
	b, err := io.ReadAll(reader)
	require.NoError(t, err, "should be able to read processed audio")

	assert.Less(t, len(b), len(data), "processed audio should be smaller than original")
}

func TestDedupeDispatch(t *testing.T) {
	// Clear cache before test
	newCache, _ := lru.New[string, bool](1000)
	dedupeCache = newCache

	meta := createTestMetadata()

	// First call should not be a duplicate
	dupe1 := dedupeDispatch(meta)
	assert.False(t, dupe1, "first call should not be duplicate")

	// Second call with same metadata should be duplicate
	dupe2 := dedupeDispatch(meta)
	assert.True(t, dupe2, "second call should be duplicate")

	// Different metadata should not be duplicate
	meta2 := createTestMetadata()
	meta2.StartTime = 1702617252 // different time
	dupe3 := dedupeDispatch(meta2)
	assert.False(t, dupe3, "different metadata should not be duplicate")
}
