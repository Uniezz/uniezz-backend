package profile

import (
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

type privacySetting struct {
	doc     string
	visible bool
	hide    func(*PublicProfile)
}

var privacySettings = map[string]privacySetting{
	"showEmail": {
		doc:     "Show the university email to other students",
		visible: false,
		hide:    func(p *PublicProfile) { p.Email = nil },
	},
	"showFaculty": {
		doc:     "Show the faculty of each study programme",
		visible: true,
		hide: func(p *PublicProfile) {
			for i := range p.Programmes {
				p.Programmes[i].Faculty = nil
			}
		},
	},
	"showYear": {
		doc:     "Show the year of study of each study programme",
		visible: true,
		hide: func(p *PublicProfile) {
			for i := range p.Programmes {
				p.Programmes[i].YearOfStudy = nil
			}
		},
	},
}

type Privacy map[string]bool

func (Privacy) Schema(huma.Registry) *huma.Schema {
	s := &huma.Schema{
		Type:                 huma.TypeObject,
		Description:          "Which profile fields other students see. A missing setting keeps its current value.",
		Properties:           map[string]*huma.Schema{},
		AdditionalProperties: false,
	}
	for _, name := range privacySettingNames() {
		setting := privacySettings[name]
		s.Properties[name] = &huma.Schema{Type: huma.TypeBoolean, Description: setting.doc, Default: setting.visible}
	}
	return s
}

func privacySettingNames() []string {
	names := make([]string, 0, len(privacySettings))
	for name := range privacySettings {
		names = append(names, name)
	}
	slices.SortFunc(names, strings.Compare)
	return names
}

func (p Privacy) withDefaults() Privacy {
	full := make(Privacy, len(privacySettings))
	for name, setting := range privacySettings {
		visible, ok := p[name]
		if !ok {
			visible = setting.visible
		}
		full[name] = visible
	}
	return full
}

func (p Privacy) apply(profile *PublicProfile) {
	for name, visible := range p.withDefaults() {
		if !visible {
			privacySettings[name].hide(profile)
		}
	}
}
