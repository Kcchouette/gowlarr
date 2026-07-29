package newznab

// Standard Newznab/Torznab category IDs (2000-8999)
var categories = map[int]string{
	// Movies
	2000: "Movies",
	2010: "Movies/Foreign",
	2020: "Movies/Other",
	2030: "Movies/SD",
	2040: "Movies/HD",
	2050: "Movies/3D",
	2060: "Movies/BluRay",
	2070: "Movies/DVD",
	2080: "Movies/Handheld",
	2090: "Movies/Phone",
	// TV
	3000: "TV",
	3010: "TV/WEB-DL",
	3020: "TV/Foreign",
	3030: "TV/SD",
	3040: "TV/HD",
	3050: "TV/Other",
	3060: "TV/Sports",
	3070: "TV/Anime",
	3080: "TV/Documentary",
	// Audio
	4000: "Audio",
	4010: "Audio/MP3",
	4020: "Audio/Full CD",
	4030: "Audio/Other",
	4040: "Audio/Foreign",
	// Books
	5000: "Books",
	5010: "Books/EBook",
	5020: "Books/Magazine",
	5030: "Books/Technical",
	5040: "Books/Comics",
	// PC
	6000: "PC",
	6010: "PC/Games",
	6020: "PC/Mac",
	6030: "PC/ISO",
	6040: "PC/0day",
	// Console
	7000: "Console",
	7010: "Console/PSP",
	7020: "Console/Xbox",
	7030: "Console/Xbox360",
	7040: "Console/NES",
	7050: "Console/SNES",
	7060: "Console/N64",
	7070: "Console/PS1",
	7080: "Console/PS2",
	7090: "Console/PS3",
	// XXX
	8000: "XXX",
	8010: "XXX/Other",
	8020: "XXX/WMV",
	8030: "XXX/XviD",
	8040: "XXX/x264",
}

// CategoryNameToID returns the Newznab category ID by its name.
// Returns 0 if the name is not found.
func CategoryNameToID(name string) int {
	for id, n := range categories {
		if n == name {
			return id
		}
	}
	return 0
}

// CategoryIDToName returns the Newznab category name by its ID.
// Returns "" if the ID is not found.
func CategoryIDToName(id int) string {
	return categories[id]
}
