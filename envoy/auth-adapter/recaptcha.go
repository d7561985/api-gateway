package main

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/tel-io/tel/v2"
)

type RecaptchaResponse struct {
	Success  bool     `json:"success"`
	Score    *float64 `json:"score"`
	Action   string   `json:"action"`
	ErrCodes []string `json:"error-codes"`
}

type RCConf struct {
	URL      string
	SecretV2 string
	SecretV3 string
	MinScore float64
}

type RecaptchaProcessor struct {
	httpCli  *http.Client
	rcURL    string
	secretV2 string
	secretV3 string
	minScore float64
	logger   *tel.Telemetry
}

func NewRecaptchaProcessor(rcConf *RCConf, logger *tel.Telemetry) *RecaptchaProcessor {
	return &RecaptchaProcessor{
		httpCli:  &http.Client{},
		rcURL:    rcConf.URL,
		secretV2: rcConf.SecretV2,
		secretV3: rcConf.SecretV3,
		minScore: rcConf.MinScore,
		logger:   logger,
	}
}

func (rp *RecaptchaProcessor) CheckRecaptcha(token string, v2 bool) bool {
	params := url.Values{}
	if v2 {
		params.Set("secret", rp.secretV2)
	} else {
		params.Set("secret", rp.secretV3)
	}
	params.Set("response", token)

	log := rp.logger.With(tel.String("token_prefix", token[:20]))

	resp, err := rp.httpCli.PostForm(rp.rcURL, params)
	if err != nil {
		log.Error("RecaptchaProcessor", tel.String("POST", rp.rcURL), tel.Error(err))
		return false
	}

	if resp.StatusCode != http.StatusOK {
		log.Error("RecaptchaProcessor failed", tel.String("POST", rp.rcURL),
			tel.String("status", resp.Status))
		return false
	}

	rcResp := RecaptchaResponse{}
	err = json.NewDecoder(resp.Body).Decode(&rcResp)
	if err != nil {
		log.Error("RecaptchaProcessor json response decode failed", tel.Error(err))
		return false
	}

	if !rcResp.Success {
		log.Warn("RecaptchaProcessor reCaptcha failed", tel.Any("resp", rcResp))
		return false
	} else {
		log.Debug("RecaptchaProcessor reCaptcha response", tel.Any("resp", rcResp))
	}

	if !v2 {
		if rcResp.Score == nil {
			log.Warn("RecaptchaProcessor <score> field does not found in v3 recaptcha", tel.Any("resp", rcResp))
			return false
		}

		if *rcResp.Score < rp.minScore {
			log.Info("RecaptchaProcessor reCaptcha v3", tel.Float64("score", *rcResp.Score))
			return false
		}
	}

	return true
}
