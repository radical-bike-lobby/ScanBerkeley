package main

// Test utilities

func createTestMetadata() Metadata {
	return Metadata{
		Freq:              772393750,
		StartTime:         1702617247,
		StopTime:          1702617252,
		Emergency:         0,
		Priority:          4,
		Mode:              0,
		Duplex:            0,
		Encrypted:         0,
		CallLength:        4,
		Talkgroup:         3105,
		TalkgroupTag:      "Berkeley PD1",
		TalkGroupDesc:     "Police Dispatch",
		TalkGroupGroupTag: "Law Dispatch",
		TalkGroupGroup:    "Berkeley",
		AudioType:         "digital",
		ShortName:         "Berkeley",
		SrcList: []Source{
			{Src: 3124119, Time: 1702617247, Pos: 0.00, Emergency: 0, SignalSystem: "", Tag: ""},
			{Src: 3113008, Time: 1702617251, Pos: 2.88, Emergency: 0, SignalSystem: "", Tag: "Dispatch"},
		},
	}
}
