package auth

import (
	"encoding/json"
	"fmt"
	"os"
)

type LocalAuthFile struct {
	AuthMode string `json:"auth_mode"`
	Tokens   struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
	} `json:"tokens"`
}

func LoadLocalAuthFile(path string) (LocalAuthFile, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LocalAuthFile{}, nil, fmt.Errorf("read local auth file: %w", err)
	}

	var file LocalAuthFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return LocalAuthFile{}, nil, fmt.Errorf("decode local auth file: %w", err)
	}
	if file.AuthMode == "" {
		return LocalAuthFile{}, nil, fmt.Errorf("local auth file missing auth_mode")
	}
	if file.Tokens.AccessToken == "" && file.Tokens.IDToken == "" {
		return LocalAuthFile{}, nil, fmt.Errorf("local auth file missing tokens")
	}

	return file, raw, nil
}
