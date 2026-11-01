package connector

import (
	"context"
	"fmt"

	"github.com/TomiCode/cnsl/internal/secret"
)

// Sealer encrypts and decrypts secret values (see package secret).
type Sealer interface {
	Seal(plain string) (string, error)
	Open(value string) (string, error)
}

// SealSecrets returns a copy with the schema's secret fields encrypted.
func (s Schema) SealSecrets(v Values, box Sealer) (Values, error) {
	return s.mapSecrets(v, box.Seal)
}

// OpenSecrets returns a copy with the schema's secret fields decrypted.
func (s Schema) OpenSecrets(v Values, box Sealer) (Values, error) {
	return s.mapSecrets(v, box.Open)
}

func (s Schema) mapSecrets(v Values, fn func(string) (string, error)) (Values, error) {
	out := Values{}
	for k, val := range v {
		out[k] = val
	}
	for _, f := range s {
		if f.Kind != "secret" || out.String(f.Key) == "" {
			continue
		}
		mapped, err := fn(fmt.Sprint(out[f.Key]))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Label, err)
		}
		out[f.Key] = mapped
	}
	return out, nil
}

// SealConfig encrypts the secret fields of a connector config before it is stored.
func (s *Service) SealConfig(typeKey string, config Values) (Values, error) {
	t, ok := Lookup(typeKey)
	if !ok {
		return nil, fmt.Errorf("unknown connector type %q", typeKey)
	}
	return t.ConfigSchema().SealSecrets(config, s.box)
}

// OpenConfig decrypts a stored connector config for use.
func (s *Service) OpenConfig(typeKey, stored string) (Values, error) {
	config, err := ParseValues(stored)
	if err != nil {
		return nil, err
	}
	t, ok := Lookup(typeKey)
	if !ok {
		return nil, fmt.Errorf("unknown connector type %q", typeKey)
	}
	return t.ConfigSchema().OpenSecrets(config, s.box)
}

// EncryptStored seals secrets that were stored before encryption existed (or by hand). Safe to run
// on every start: already sealed values are left alone. Returns how many connectors changed.
func (s *Service) EncryptStored(ctx context.Context) (int, error) {
	conns, err := s.store.Connectors(ctx)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, c := range conns {
		t, ok := Lookup(c.Type)
		if !ok {
			continue
		}
		config, err := ParseValues(c.Config)
		if err != nil {
			continue
		}
		plain := false
		for _, f := range t.ConfigSchema() {
			if f.Kind == "secret" && config.String(f.Key) != "" && !secret.IsSealed(config.String(f.Key)) {
				plain = true
			}
		}
		if !plain {
			continue
		}
		sealed, err := t.ConfigSchema().SealSecrets(config, s.box)
		if err != nil {
			return changed, err
		}
		c.Config = sealed.JSON()
		if err := s.store.SaveConnector(ctx, &c); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}
