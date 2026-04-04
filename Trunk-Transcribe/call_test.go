package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		name        string
		call        Call
		expectValid bool
		expectErr   string
	}{
		{
			name: "valid call",
			call: Call{
				Audio:     make([]byte, 100),
				DateTime:  time.Now(),
				System:    1,
				Talkgroup: 1,
			},
			expectValid: true,
			expectErr:   "",
		},
		{
			name: "no audio",
			call: Call{
				Audio:     make([]byte, 44),
				DateTime:  time.Now(),
				System:    1,
				Talkgroup: 1,
			},
			expectValid: false,
			expectErr:   "no audio",
		},
		{
			name: "no talkgroup",
			call: Call{
				Audio:     make([]byte, 100),
				DateTime:  time.Now(),
				System:    1,
				Talkgroup: 0,
			},
			expectValid: false,
			expectErr:   "no talkgroup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := tt.call
			valid, err := call.IsValid()

			assert.Equal(t, tt.expectValid, valid, "validity should match")

			if tt.expectErr != "" {
				assert.Error(t, err, "should return error")
				assert.Contains(t, err.Error(), tt.expectErr, "error message should match")
			} else {
				assert.NoError(t, err, "should not return error")
			}
		})
	}
}
