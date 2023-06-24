package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/tel-io/tel/v2"
)

type RateLimitManager struct {
	logger *tel.Telemetry
	rlConf map[string]*rateLimitConf

	mx         *sync.Mutex
	rlProgress map[string]*rateLimitProgress
}

type rateLimitProgress struct {
	IPRate    map[string]int
	PeriodEnd time.Time
}

func NewRateLimitManager(conf *APIConf, logger *tel.Telemetry) *RateLimitManager {
	rlConf := make(map[string]*rateLimitConf)

	for _, api := range conf.APIsDescr {
		for _, method := range api.Methods {
			fullPath := fmt.Sprintf("%s/%s", api.Name, method.Name)

			if method.Auth != nil && method.Auth.RateLimit != nil {
				logger.Info("add rate limit config",
					tel.String("method", fullPath), tel.Any("limit", method.Auth.RateLimit))
				rlConf[fullPath] = method.Auth.RateLimit
			}
		}
	}

	return &RateLimitManager{
		logger: logger,
		rlConf: rlConf,

		rlProgress: make(map[string]*rateLimitProgress),
		mx:         &sync.Mutex{},
	}
}

func (rlm *RateLimitManager) Check(ip, method string) bool {
	cfg, ok := rlm.rlConf[method]
	if !ok {
		//no need rate limit
		return true
	}

	rlm.logger.Debug("checking rate limit for method=%s and IP=%s",
		tel.String("method", method), tel.String("ip", ip))

	rlm.mx.Lock()
	defer rlm.mx.Unlock()

	var progress *rateLimitProgress

	now := time.Now()
	progress, ok = rlm.rlProgress[method]
	if !ok || now.After(progress.PeriodEnd) {
		rlm.logger.Debug("create new rate limit progress", tel.String("method", method))

		progress = &rateLimitProgress{
			IPRate:    make(map[string]int),
			PeriodEnd: now.Add(cfg.Period),
		}

		rlm.rlProgress[method] = progress
	}

	progress.IPRate[ip] += 1
	if progress.IPRate[ip] > cfg.Count {
		rlm.logger.Error("rate limit reached",
			tel.String("method", method), tel.String("ip", ip))

		if cfg.Delay > 0 && progress.IPRate[ip] > cfg.Count+2 {
			// NOTICE: we should skip first rate limit for faster reCaptcha v2 display and validate
			time.Sleep(cfg.Delay)
		}

		return false
	}

	return true
}

func (rlm *RateLimitManager) Reset(ip, method string) {
	rlm.mx.Lock()
	defer rlm.mx.Unlock()

	progress, ok := rlm.rlProgress[method]
	if !ok {
		return
	}

	delete(progress.IPRate, ip)
}
