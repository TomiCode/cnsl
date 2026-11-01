package dashboard

import (
	"context"
	"fmt"

	"github.com/TomiCode/cnsl/internal/dashboard/gen"
	"github.com/TomiCode/cnsl/internal/i18n"
	"github.com/TomiCode/cnsl/internal/icons"
	"github.com/TomiCode/cnsl/internal/store"
)

func toLanguages() []struct {
	Code string `json:"code"`
	Name string `json:"name"`
} {
	langs := i18n.Languages()
	out := make([]struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}, len(langs))
	for i, l := range langs {
		out[i].Code, out[i].Name = l.Code, l.Name
	}
	return out
}

func (s *Server) settings(ctx context.Context) (gen.Settings, error) {
	lang, err := s.store.Setting(ctx, store.SettingLanguage, i18n.Default)
	return gen.Settings{Language: lang}, err
}

func (s *Server) GetSettings(ctx context.Context, _ gen.GetSettingsRequestObject) (gen.GetSettingsResponseObject, error) {
	out, err := s.settings(ctx)
	return gen.GetSettings200JSONResponse{SettingsJSONResponse: gen.SettingsJSONResponse(out)}, err
}

func (s *Server) UpdateSettings(ctx context.Context, req gen.UpdateSettingsRequestObject) (gen.UpdateSettingsResponseObject, error) {
	if !i18n.Valid(req.Body.Language) {
		return gen.UpdateSettings422ApplicationProblemPlusJSONResponse{ProblemApplicationProblemPlusJSONResponse: invalidProblem(fmt.Errorf("unknown language %q", req.Body.Language))}, nil
	}
	before, err := s.settings(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.store.SetSetting(ctx, store.SettingLanguage, req.Body.Language); err != nil {
		return nil, err
	}
	if before.Language != req.Body.Language {
		screens, err := s.store.Screens(ctx)
		if err != nil {
			return nil, err
		}
		for _, sc := range screens {
			if sc.Language == "" && sc.Kind != "image" {
				s.scheduler.Request(sc.ID, "settings")
			}
		}
	}
	out, err := s.settings(ctx)
	return gen.UpdateSettings200JSONResponse{SettingsJSONResponse: gen.SettingsJSONResponse(out)}, err
}

func (s *Server) ListIcons(_ context.Context, _ gen.ListIconsRequestObject) (gen.ListIconsResponseObject, error) {
	all, err := icons.All()
	if err != nil {
		return nil, err
	}
	out := make(gen.ListIcons200JSONResponse, len(all))
	for i, ic := range all {
		out[i] = gen.IconInfo{Name: ic.Name, Set: gen.IconInfoSet(ic.Set)}
		if len(ic.Tags) > 0 {
			tags := ic.Tags
			out[i].Tags = &tags
		}
	}
	return out, nil
}
