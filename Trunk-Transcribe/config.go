package main

import (
	"log"
	"regexp"
	"strings"
)

var (
	puncRegex    = regexp.MustCompile("[\\.\\!\\?;]\\s+")
	wordsRegex   = regexp.MustCompile("[a-zA-Z0-9_-]+")
	numericRegex = regexp.MustCompile("[0-9]+")
	modeString   = `(auto|car|driver|vehicle|bike|pedestrian|ped|bicycle|cyclist|bicyclist|pavement)s?`
	versusRegex  = regexp.MustCompile(modeString + `.+(vs|versus|verses)(\.)?.+` + modeString)
	streets      = []string{"Acton", "Ada", "Addison", "Adeline", "Alcatraz", "Allston", "Ashby", "Bancroft", "Benvenue", "Berryman", "Blake", "Bonar", "Bonita", "Bowditch", "Buena", "California", "Camelia", "Carleton", "Carlotta", "Cedar", "Center", "Channing", "Chestnut", "Claremont", "Codornices", "College", "Cragmont", "Delaware", "Derby", "Dwight", "Eastshore", "Edith", "Elmwood", "Euclid", "Francisco", "Fresno", "Gilman", "Grizzly", "Harrison", "Hearst", "Heinz", "Henry", "Hillegass", "Holly", "Hopkins", "Josephine", "Kains", "Keoncrest", "King", "LeConte", "LeRoy", "Hilgard", "Mabel", "Marin", "Martin", "MLK", "Milvia", "Monterey", "Napa", "Neilson", "Oregon", "Parker", "Piedmont", "Posen", "Rose", "Russell", "Sacramento", "San Pablo", "Santa", "Fe", "Shattuck", "Solano", "Sonoma", "Spruce", "Telegraph", "Alameda", "Thousand", "Oaks", "University", "Vine", "Virginia", "Ward", "Woolsey"}
	modifiers    = []string{"street", "boulevard", "road", "path", "way", "avenue", "highway"}
	terms        = []string{"bike", "bicycle", "pedestrian", "vehicle", "injury", "victim", "versus", "transport", "concious", "breathing", "alta bates", "highland", "BFD", "Adam", "ID tech", "ring on three", "code 2", "code 3", "code 4", "code 34", "en route", "case number", "berry brothers", "rita run", "DBF", "Falck", "Falck on order", "this is Falck", "Flock camera", "10-four", "10-4", "10 four", "His Lordships", "Cesar Chavez Park", "10-9 your traffic", "copy", "tow", "the beat"}

	defaultChannelID = BERKELEY // #scanner-dispatches

	talkgroupToChannel = map[int64][]SlackChannelID{
		2100:                          []SlackChannelID{BERKELEY},
		2105:                          []SlackChannelID{BERKELEY},
		2106:                          []SlackChannelID{BERKELEY},
		2107:                          []SlackChannelID{BERKELEY},
		2108:                          []SlackChannelID{BERKELEY},
		2109:                          []SlackChannelID{BERKELEY},
		2110:                          []SlackChannelID{BERKELEY},
		2111:                          []SlackChannelID{BERKELEY},
		2112:                          []SlackChannelID{BERKELEY},
		2671:                          []SlackChannelID{BERKELEY},
		2672:                          []SlackChannelID{BERKELEY},
		2691:                          []SlackChannelID{BERKELEY},
		2692:                          []SlackChannelID{BERKELEY},
		2711:                          []SlackChannelID{BERKELEY},
		2712:                          []SlackChannelID{BERKELEY},
		3100:                          []SlackChannelID{BERKELEY},
		3105:                          []SlackChannelID{BERKELEY},
		3106:                          []SlackChannelID{BERKELEY},
		3108:                          []SlackChannelID{BERKELEY},
		3110:                          []SlackChannelID{BERKELEY},
		3112:                          []SlackChannelID{BERKELEY},
		4100:                          []SlackChannelID{BERKELEY},
		4105:                          []SlackChannelID{BERKELEY},
		4106:                          []SlackChannelID{BERKELEY},
		4107:                          []SlackChannelID{BERKELEY},
		4108:                          []SlackChannelID{BERKELEY},
		4109:                          []SlackChannelID{BERKELEY},
		4110:                          []SlackChannelID{BERKELEY},
		4111:                          []SlackChannelID{BERKELEY},
		4112:                          []SlackChannelID{BERKELEY},
		ALTA_BATES_HOSPITAL_TALKGROUP: []SlackChannelID{HOSPITALS},
		3605:                          []SlackChannelID{UCPD},
		3606:                          []SlackChannelID{UCPD},
		3608:                          []SlackChannelID{UCPD},
		3609:                          []SlackChannelID{UCPD},
		3055:                          []SlackChannelID{ALBANY},
		3056:                          []SlackChannelID{ALBANY},
		3057:                          []SlackChannelID{ALBANY},
		3058:                          []SlackChannelID{ALBANY},
		3059:                          []SlackChannelID{ALBANY},
		2050:                          []SlackChannelID{ALBANY},
		2055:                          []SlackChannelID{ALBANY},
		2056:                          []SlackChannelID{ALBANY},
		2057:                          []SlackChannelID{ALBANY},
		2058:                          []SlackChannelID{ALBANY},
		2059:                          []SlackChannelID{ALBANY},
		4055:                          []SlackChannelID{ALBANY},
		3155:                          []SlackChannelID{EMERYVILLE},
		3156:                          []SlackChannelID{EMERYVILLE},
		3157:                          []SlackChannelID{EMERYVILLE},
		4155:                          []SlackChannelID{EMERYVILLE},
		3405:                          []SlackChannelID{OAKLAND},
		3406:                          []SlackChannelID{OAKLAND},
		3407:                          []SlackChannelID{OAKLAND},
		3408:                          []SlackChannelID{OAKLAND},
		3409:                          []SlackChannelID{OAKLAND},
		3410:                          []SlackChannelID{OAKLAND},
		3411:                          []SlackChannelID{OAKLAND},
		3418:                          []SlackChannelID{OAKLAND},
		3419:                          []SlackChannelID{OAKLAND},
		3420:                          []SlackChannelID{OAKLAND},
		3421:                          []SlackChannelID{OAKLAND},
		3422:                          []SlackChannelID{OAKLAND},
		3423:                          []SlackChannelID{OAKLAND},
		3424:                          []SlackChannelID{OAKLAND},
		3425:                          []SlackChannelID{OAKLAND},
		3426:                          []SlackChannelID{OAKLAND},
		3428:                          []SlackChannelID{OAKLAND},
		3429:                          []SlackChannelID{OAKLAND},
		3447:                          []SlackChannelID{OAKLAND},
		3448:                          []SlackChannelID{OAKLAND},
		2400:                          []SlackChannelID{OAKLAND_FIRE},
		2405:                          []SlackChannelID{OAKLAND_FIRE},
		2406:                          []SlackChannelID{OAKLAND_FIRE},
		2407:                          []SlackChannelID{OAKLAND_FIRE},
		2408:                          []SlackChannelID{OAKLAND_FIRE},
		2409:                          []SlackChannelID{OAKLAND_FIRE},
		2410:                          []SlackChannelID{OAKLAND_FIRE},
		2411:                          []SlackChannelID{OAKLAND_FIRE},
		2412:                          []SlackChannelID{OAKLAND_FIRE},
		2413:                          []SlackChannelID{OAKLAND_FIRE},
		2414:                          []SlackChannelID{OAKLAND_FIRE},
		2416:                          []SlackChannelID{OAKLAND_FIRE},
		2417:                          []SlackChannelID{OAKLAND_FIRE},
		2434:                          []SlackChannelID{OAKLAND_FIRE},
		2436:                          []SlackChannelID{OAKLAND_FIRE},
		4405:                          []SlackChannelID{OAKLAND},
		4407:                          []SlackChannelID{OAKLAND},
		4415:                          []SlackChannelID{OAKLAND},
		4421:                          []SlackChannelID{OAKLAND},
		4422:                          []SlackChannelID{OAKLAND},
		4423:                          []SlackChannelID{OAKLAND},
		CHILDRENS_HOSPITAL_TALKGROUP:  []SlackChannelID{HOSPITALS_TRAUMA},        // Childrens Hospital
		HIGHLAND_HOSPITAL_TALKGROUP:   []SlackChannelID{HOSPITALS_TRAUMA}, // Highland Hospital
		5516:                          []SlackChannelID{HOSPITALS},               // Summit Hospital
		5512:                          []SlackChannelID{HOSPITALS},
	}

	// Determines slack channel to send to from the passed metadata
	channelResolver = func(meta Metadata) []SlackChannelID {
		channels, ok := talkgroupToChannel[meta.Talkgroup]
		if ok {
			return channels
		}

		talkgroupGroup := strings.ToLower(meta.TalkGroupGroup)
		talkgroupTag := strings.ToLower(meta.TalkgroupTag)
		switch talkgroupGroup {		
		case "al co sheriff":
			channels = append(channels, ALAMEDA_COUNTY)
		case "al co ems":
			channels = append(channels, ALAMEDA_COUNTY_EMS)
		case "al co fire":
			channels = append(channels, ALAMEDA_COUNTY_FIRE)
		case "al co services":
			channels = append(channels, ALAMEDA_COUNTY_SERVICES)
		case "alameda":
			channels = append(channels, ALAMEDA)
		case "amr (ccc)":
			channels = append(channels, AMR_CCC)
		case "berkeley":
			channels = append(channels, BERKELEY, BERKELEY_SECONDARY)
		case "oakland":
			switch talkgroupTag {
			case "fire dispatch":
				channels = append(channels, OAKLAND, OAKLAND_FIRE_SECONDARY)
			default:
				channels = append(channels, OAKLAND, OAKLAND_SECONDARY)
			}
		case "east bay regional park district":
			channels = append(channels, EAST_BAY_REGIONAL_PARK)
		case "falck ambulance":
			channels = append(channels, FALCK_AMBULANCE)
		case "piedmont":
			channels = append(channels, PIEDMONT)
		case "albany":
			channels = append(channels, ALBANY)
		case "emeryville":
			channels = append(channels, EMERYVILLE)
		case "hayward":
			channels = append(channels, HAYWARD)
		case "bart":
			channels = append(channels, BART)
		case "us coast guard":
			channels = append(channels, US_COAST_GUARD)
		default:
			log.Printf("Could not resolve channel for talkgroup: %s, %s, %d", meta.TalkGroupGroup, meta.TalkgroupTag, meta.Talkgroup)
		}
		return channels
	}
)

//slack user id to keywords map
// supports regex

var notifsMap = map[SlackUserID][]Notifs{
	EMILIE: []Notifs{
		{
			Include:  []string{"auto ped", "auto-ped", "autoped", "autobike", "auto bike", "auto bicycle", "auto-bike", "auto-bicycle", "hit and run", "1071", "GSW", "loud reports", "211", "highland", "catalytic", "apple", "261", "code 3", "10-15", "beeper", "1053", "1054", "1055", "1080", "1199", "DBF", "Code 33", "1180", "215", "220", "243", "244", "243", "288", "451", "288A", "243", "207", "212.5", "1079", "1067", "accident", "collision", "fled", "homicide", "fait", "fate", "injuries", "conscious", "responsive", "shooting", "shoot", "coroner", "weapon", "weapons", "gun", "flock", "spikes", "challenging", "beeper", "cage", "register", "1033 frank", "1033f", "1033", "10-33 frank", "pursuit", "frank"},
			NotRegex: regexp.MustCompile("no (weapon|gun)s?"),
			Regex:    versusRegex,
			Channels: []SlackChannelID{BERKELEY, UCPD},
		},
		{
			Include: []string{"trauma", "trauma activation"},
		},
	},
	NAVEEN: []Notifs{{
		Include:  []string{"hit and run", "auto ped", "auto-ped", "autoped", "autobike", "auto bicycle", "auto-bike", "auto-bicycle", "Rose St", "Rose Street", "Ruth Acty", "King Middle"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{BERKELEY, UCPD, ALBANY, EMERYVILLE},
	}},
	MARC: []Notifs{{
		Include:  []string{"hit and run", "autobike", "auto bike", "auto bicycle", "auto bicyclist", "auto ped", "auto-ped", "autoped"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{BERKELEY, UCPD},
	}},
	JOSE: []Notifs{{
		Include:  []string{"accident", "collision", "crash", "crashed", "crashes"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{OAKLAND},
	}},
	STEPHAN: []Notifs{{
		Include:  []string{"GSW", "Active Shooter", "Shots Fired", "Pursuit", "Structure Fire", "Shooting", "Shooter", "Shots", "Code 33", "glock"},
		NotRegex: regexp.MustCompile("no (weapon|gun)s?"),
		Channels: []SlackChannelID{BERKELEY, UCPD},
	}},
	HELEN: []Notifs{{
		Include:  []string{"hit and run", "autobike", "auto bike", "auto bicycle", "auto bicyclist", "auto ped", "auto-ped", "autoped", "marin", "hopkins", "el dorado"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{BERKELEY},
	}},
	TAJ: []Notifs{{
		Include:  []string{"autobike", "auto bike", "auto bicycle", "auto bicyclist", "auto ped", "auto-ped", "autoped"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{BERKELEY},
	}},
}
