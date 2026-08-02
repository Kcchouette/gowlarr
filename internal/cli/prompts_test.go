package cli

import (
	"reflect"
	"testing"

	"github.com/Kcchouette/cardigann-go/definition"
)

func japanfanLikeDef() definition.IndexerDefinition {
	return definition.IndexerDefinition{
		Settings: []definition.Setting{
			{Name: "username", Type: "text", Label: "Username"},
			{Name: "password", Type: "password", Label: "Password"},
			{Name: "info_sitelink", Type: "info", Label: "Site", Default: "https://japanfan.org/phpBB3/"},
		},
	}
}

func abnormalLikeDef() definition.IndexerDefinition {
	return definition.IndexerDefinition{
		Settings: []definition.Setting{
			{Name: "apikey", Type: "text", Label: "APIKey"},
			{Name: "info_key", Type: "info", Label: "About your API key", Default: "Find your API key on your Parameters page."},
			{Name: "apiurl", Type: "text", Label: "API URL", Default: "api.abn.lol"},
			{Name: "freeleech", Type: "checkbox", Label: "Freeleech only", Default: "false"},
			{Name: "multilanguage", Type: "select", Label: "Language", Default: "FRENCH"},
			{Name: "sort", Type: "select", Label: "Sort", Default: "Created"},
		},
	}
}

func TestRequiredSettings(t *testing.T) {
	tests := []struct {
		name     string
		def      definition.IndexerDefinition
		provided map[string]string
		want     []string
	}{
		{
			"japanfan nothing provided",
			japanfanLikeDef(),
			nil,
			[]string{"username", "password"},
		},
		{
			"japanfan username provided",
			japanfanLikeDef(),
			map[string]string{"username": "alice"},
			[]string{"password"},
		},
		{
			"japanfan all provided",
			japanfanLikeDef(),
			map[string]string{"username": "alice", "password": "secret"},
			nil,
		},
		{
			"japanfan empty value counts as provided",
			japanfanLikeDef(),
			map[string]string{"password": ""},
			[]string{"username"},
		},
		{
			"abnormal only apikey required",
			abnormalLikeDef(),
			nil,
			[]string{"apikey"},
		},
		{
			"abnormal apikey provided",
			abnormalLikeDef(),
			map[string]string{"apikey": "k"},
			nil,
		},
		{
			"select without default is not required",
			definition.IndexerDefinition{Settings: []definition.Setting{
				{Name: "mode", Type: "select", Label: "Mode"},
			}},
			nil,
			nil,
		},
		{
			"unknown or empty type is not required",
			definition.IndexerDefinition{Settings: []definition.Setting{
				{Name: "weird", Type: "custom"},
				{Name: "empty", Type: ""},
			}},
			nil,
			nil,
		},
		{
			"empty settings list",
			definition.IndexerDefinition{},
			nil,
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requiredSettings(tt.def, tt.provided)
			names := make([]string, 0, len(got))
			for _, s := range got {
				names = append(names, s.Name)
			}
			if len(names) == 0 {
				names = nil
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Errorf("requiredSettings = %v, want %v", names, tt.want)
			}
		})
	}
}
