package main

import (
	"log"
	"net/http"

	"github.com/maxsupermanhd/lac"
)

func telemetryStartHttpServer(c *lac.ConfSubtree) {
	m := http.NewServeMux()
	m.HandleFunc("/skincache", wrapApiCall(telemetryGetSkinCacheCount))
	go log.Println(http.ListenAndServe(c.GetDSString("127.0.0.1:9271", "ListenAddr"), m))
}

func telemetryGetSkinCacheCount(w http.ResponseWriter, r *http.Request) (int, any) {
	count, size, err := sc.GetCachedSkinsCountSize()
	if err != nil {
		return 500, err
	}
	return 200, map[string]any{
		"count": count,
		"size":  size,
	}
}
