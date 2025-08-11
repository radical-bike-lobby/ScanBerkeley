package main

import (
	"regexp"
	"time"
)

var (
	puncRegex    = regexp.MustCompile("[\\.\\!\\?;]\\s+")
	wordsRegex   = regexp.MustCompile("[a-zA-Z0-9_-]+")
	numericRegex = regexp.MustCompile("[0-9]+")
	modeString   = `(auto|car|driver|vehicle|bike|pedestrian|ped|bicycle|cyclist|bicyclist|pavement)s?`
	versusRegex  = regexp.MustCompile(modeString + `.+(vs|versus|verses)(\.)?.+` + modeString)
	streets      = []string{"Acton", "Ada", "Addison", "Adeline", "Alcatraz", "Allston", "Ashby", "Bancroft", "Benvenue", "Berryman", "Blake", "Bonar", "Bonita", "Bowditch", "Buena", "California", "Camelia", "Carleton", "Carlotta", "Cedar", "Center", "Channing", "Chestnut", "Claremont", "Codornices", "College", "Cragmont", "Delaware", "Derby", "Dwight", "Eastshore", "Edith", "Elmwood", "Euclid", "Francisco", "Fresno", "Gilman", "Grizzly", "Harrison", "Hearst", "Heinz", "Henry", "Hillegass", "Holly", "Hopkins", "Josephine", "Kains", "Keoncrest", "King", "LeConte", "LeRoy", "Hilgard", "Mabel", "Marin", "Martin", "MLK", "Milvia", "Monterey", "Napa", "Neilson", "Oregon", "Parker", "Piedmont", "Posen", "Rose", "Russell", "Sacramento", "San Pablo", "Santa", "Fe", "Shattuck", "Solano", "Sonoma", "Spruce", "Telegraph", "Alameda", "Thousand", "Oaks", "University", "Vine", "Virginia", "Ward", "Woolsey"}
	modifiers    = []string{"street", "boulevard", "road", "path", "way", "avenue", "highway"}
	terms        = []string{"bike", "bicycle", "pedestrian", "vehicle", "injury", "victim", "versus", "transport", "concious", "breathing", "alta bates", "highland", "BFD", "Adam", "ID tech", "ring on three", "code 2", "code 3", "code 4", "code 34", "en route", "case number", "berry brothers", "rita run", "DBF", "Falck", "Falck on order", "this is Falck", "Flock camera", "10-four", "10-4", "10 four", "His Lordships", "Cesar Chavez Park", "10-9 your traffic", "copy", "tow", "kill the beeper"}

	defaultChannelID = BERKELEY // #scanner-dispatches

	talkgroupToChannel = map[int64]SlackChannelID{
		3605:               UCPD, // UCB PD1 : #scanner-dispatches-ucpd
		3606:               UCPD, // UCB PD2 : #scanner-dispatches-ucpd
		3608:               UCPD, // UCB PD4 : #scanner-dispatches-ucpd
		3609:               UCPD, // UCB PD5 : #scanner-dispatches-ucpd
		3055:               ALBANY,
		3056:               ALBANY,
		3057:               ALBANY,
		3058:               ALBANY,
		3059:               ALBANY,
		2050:               ALBANY,
		2055:               ALBANY,
		2056:               ALBANY,
		2057:               ALBANY,
		2058:               ALBANY,
		2059:               ALBANY,
		4055:               ALBANY,
		3155:               EMERYVILLE,
		3156:               EMERYVILLE,
		3157:               EMERYVILLE,
		4155:               EMERYVILLE,
		3405:               OAKLAND,
		3406:               OAKLAND,
		3407:               OAKLAND,
		3408:               OAKLAND,
		3409:               OAKLAND,
		3410:               OAKLAND,
		3411:               OAKLAND,
		3418:               OAKLAND,
		3419:               OAKLAND,
		3420:               OAKLAND,
		3421:               OAKLAND,
		3422:               OAKLAND,
		3423:               OAKLAND,
		3424:               OAKLAND,
		3425:               OAKLAND,
		3426:               OAKLAND,
		3428:               OAKLAND,
		3429:               OAKLAND,
		3447:               OAKLAND,
		3448:               OAKLAND,
		2400:               OAKLAND,
		2405:               OAKLAND,
		2406:               OAKLAND,
		2407:               OAKLAND,
		2408:               OAKLAND,
		2409:               OAKLAND,
		2410:               OAKLAND,
		2411:               OAKLAND,
		2412:               OAKLAND,
		2413:               OAKLAND,
		2414:               OAKLAND,
		2416:               OAKLAND,
		2417:               OAKLAND,
		2434:               OAKLAND,
		2436:               OAKLAND,
		4405:               OAKLAND,
		4407:               OAKLAND,
		4415:               OAKLAND,
		4421:               OAKLAND,
		4422:               OAKLAND,
		4423:               OAKLAND,
		CHILDRENS_HOSPITAL: OAKLAND, // Childrens Hospital
		HIGHLAND_HOSPITAL:  OAKLAND, // Highland Hospital
		5516:               OAKLAND, // Summit Hospital
		5512:               OAKLAND,
	}

	location *time.Location
)

//slack user id to keywords map
// supports regex

var notifsMap = map[SlackUserID][]Notifs{
	EMILIE: []Notifs{
		{
			Include:    []string{"auto ped", "auto-ped", "autoped", "autobike", "auto bike", "auto bicycle", "auto-bike", "auto-bicycle", "hit and run", "1071", "GSW", "loud reports", "211", "highland", "catalytic", "apple", "261", "code 3", "10-15", "beeper", "1053", "1054", "1055", "1080", "1199", "DBF", "Code 33", "1180", "215", "220", "243", "244", "243", "288", "451", "288A", "243", "207", "212.5", "1079", "1067", "accident", "collision", "fled", "homicide", "fait", "fate", "injuries", "conscious", "responsive", "shooting", "shoot", "coroner", "weapon", "weapons", "gun", "flock", "spikes", "challenging", "beeper", "cage", "tom", "register", "1033 frank", "1033f", "1033", "10-33 frank", "pursuit", "frank"},
			NotRegex:   regexp.MustCompile("no (weapon|gun)s?"),
			Regex:      versusRegex,
			Channels:   []SlackChannelID{BERKELEY, UCPD},
		},
		{
			Include:    []string{"trauma", "trauma activation"},
			TalkGroups: []TalkGroupID{HIGHLAND_HOSPITAL, CHILDRENS_HOSPITAL},
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
  Include: []string{"autobike", "auto bike", "auto bicycle", "auto bicyclist", "auto ped", "auto-ped", "autoped"},
  Regex: versusRegex,
  Channels: []SlackChannelID{BERKELEY},
 }},
}
