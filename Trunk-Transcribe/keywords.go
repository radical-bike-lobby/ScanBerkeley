package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// UserNotifConfig holds the editable notification preferences for a user.
type UserNotifConfig struct {
	Keywords []string         `json:"keywords"`
	Channels []SlackChannelID `json:"channels"`
}

// KeywordsStore is an in-memory store of per-user notification configs,
// backed by R2 for persistence.
type KeywordsStore struct {
	mu      sync.RWMutex
	configs map[SlackUserID]UserNotifConfig
}

var userDisplayNames = map[SlackUserID]string{
	EMILIE:  "Emilie",
	NAVEEN:  "Naveen",
	MARC:    "Marc",
	JOSE:    "Jose",
	STEPHAN: "Stephan",
	HELEN:   "Helen",
	TAJ:     "Taj",
}

var channelDisplayNames = map[SlackChannelID]string{
	BERKELEY:        "Berkeley PD",
	UCPD:            "UCPD",
	OAKLAND:         "Oakland PD",
	ALBANY:          "Albany PD",
	EMERYVILLE:      "Emeryville PD",
	OAKLAND_FIRE:    "Oakland Fire",
	HAYWARD:         "Hayward",
	ALAMEDA_COUNTY:  "Alameda County",
	BART:            "BART",
	HOSPITALS:       "Hospitals",
	HOSPITALS_TRAUMA: "Hospitals (Trauma)",
}

var editableChannels = []SlackChannelID{
	BERKELEY, UCPD, OAKLAND, ALBANY, EMERYVILLE,
	OAKLAND_FIRE, HAYWARD, ALAMEDA_COUNTY, BART,
	HOSPITALS, HOSPITALS_TRAUMA,
}

var orderedUsers = []SlackUserID{EMILIE, NAVEEN, MARC, JOSE, STEPHAN, HELEN, TAJ}

// newKeywordsStore initializes a KeywordsStore from the hardcoded notifsMap defaults.
func newKeywordsStore() *KeywordsStore {
	ks := &KeywordsStore{
		configs: make(map[SlackUserID]UserNotifConfig),
	}
	for userID, notifs := range notifsMap {
		if len(notifs) == 0 {
			continue
		}
		ks.configs[userID] = UserNotifConfig{
			Keywords: notifs[0].Include,
			Channels: notifs[0].Channels,
		}
	}
	return ks
}

func (ks *KeywordsStore) Get(userID SlackUserID) UserNotifConfig {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.configs[userID]
}

func (ks *KeywordsStore) Set(userID SlackUserID, cfg UserNotifConfig) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.configs[userID] = cfg
}

// MergedNotifsMap returns a copy of notifsMap with the first Notifs entry's
// Include and Channels replaced by the stored configs from this store.
func (ks *KeywordsStore) MergedNotifsMap() map[SlackUserID][]Notifs {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	merged := make(map[SlackUserID][]Notifs, len(notifsMap))
	for userID, notifs := range notifsMap {
		if len(notifs) == 0 {
			merged[userID] = notifs
			continue
		}
		cfg, ok := ks.configs[userID]
		if !ok {
			merged[userID] = notifs
			continue
		}
		newNotifs := make([]Notifs, len(notifs))
		copy(newNotifs, notifs)
		newNotifs[0].Include = cfg.Keywords
		newNotifs[0].Channels = cfg.Channels
		merged[userID] = newNotifs
	}
	return merged
}

// LoadFromR2 loads user configs from R2. If the file doesn't exist, it is a no-op.
func (ks *KeywordsStore) LoadFromR2(ctx context.Context, sess *session.Session) {
	svc := s3.New(sess)
	result, err := svc.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String("scanner-berkeley"),
		Key:    aws.String("config/user_keywords.json"),
	})
	if err != nil {
		log.Printf("[KeywordsStore] LoadFromR2: %v (no-op if file not found)", err)
		return
	}
	defer result.Body.Close()

	var configs map[SlackUserID]UserNotifConfig
	if err := json.NewDecoder(result.Body).Decode(&configs); err != nil {
		log.Printf("[KeywordsStore] LoadFromR2 decode error: %v", err)
		return
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()
	for userID, cfg := range configs {
		ks.configs[userID] = cfg
	}
	log.Printf("[KeywordsStore] Loaded %d user configs from R2", len(configs))
}

// SaveToR2 persists the current configs to R2.
func (ks *KeywordsStore) SaveToR2(ctx context.Context, uploader *s3manager.Uploader) error {
	ks.mu.RLock()
	data, err := json.Marshal(ks.configs)
	ks.mu.RUnlock()
	if err != nil {
		return err
	}

	_, err = uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket:      aws.String("scanner-berkeley"),
		Key:         aws.String("config/user_keywords.json"),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	return err
}

// Template data types for the keywords editor UI.

type UserTemplateEntry struct {
	ID              SlackUserID
	DisplayName     string
	Keywords        string
	ChannelSelected map[SlackChannelID]bool
}

type ChannelEntry struct {
	ID          SlackChannelID
	DisplayName string
}

type KeywordsTemplateData struct {
	Users     []UserTemplateEntry
	Channels  []ChannelEntry
	SavedName string
}

// buildKeywordsTemplateData constructs the template data from the current store state.
func buildKeywordsTemplateData(ks *KeywordsStore, savedUserID string) KeywordsTemplateData {
	savedName := ""
	if savedUserID != "" {
		savedName = userDisplayNames[SlackUserID(savedUserID)]
	}

	var channels []ChannelEntry
	for _, chID := range editableChannels {
		channels = append(channels, ChannelEntry{
			ID:          chID,
			DisplayName: channelDisplayNames[chID],
		})
	}

	var users []UserTemplateEntry
	for _, userID := range orderedUsers {
		cfg := ks.Get(userID)
		selected := make(map[SlackChannelID]bool, len(cfg.Channels))
		for _, ch := range cfg.Channels {
			selected[ch] = true
		}
		users = append(users, UserTemplateEntry{
			ID:              userID,
			DisplayName:     userDisplayNames[userID],
			Keywords:        strings.Join(cfg.Keywords, "\n"),
			ChannelSelected: selected,
		})
	}

	return KeywordsTemplateData{
		Users:     users,
		Channels:  channels,
		SavedName: savedName,
	}
}
