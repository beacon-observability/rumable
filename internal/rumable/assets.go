package rumable

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed assets/*
var assets embed.FS

type SDKInfo struct {
	Version string `json:"version"`
	Source  string `json:"source"`
	SHA256  string `json:"sha256"`
}

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func sdk() ([]byte, SDKInfo, error) {
	var info SDKInfo
	b, err := assets.ReadFile("assets/dataflux-rum.js")
	if err != nil {
		return nil, info, err
	}
	m, err := assets.ReadFile("assets/sdk.json")
	if err != nil {
		return nil, info, err
	}
	if err = json.Unmarshal(m, &info); err != nil {
		return nil, info, err
	}
	if hash(b) != info.SHA256 {
		return nil, info, fmt.Errorf("embedded SDK checksum mismatch")
	}
	return b, info, nil
}

func sdkLocation(s Site, info SDKInfo) string {
	if s.SDK.Source == "cdn" {
		if s.SDK.URL != "" {
			return s.SDK.URL
		}
		return info.Source
	}
	return s.assetPrefix() + "sdk." + info.SHA256 + ".js"
}

func bootstrap(s Site, sdkURL string, info SDKInfo) []byte {
	b, _ := json.Marshal(s.RUM.sdkConfig())
	if s.SDK.Loading == "sync" {
		return []byte(strings.NewReplacer("RUM_CONFIG", string(b), "START_REPLAY", fmt.Sprint(s.RUM.startsReplay())).Replace(`(function(w){
"use strict";
if(w.__RUMABLE__){return;}
var state=w.__RUMABLE__={status:"loading"};
try{
if(!w.DATAFLUX_RUM||typeof w.DATAFLUX_RUM.init!=="function"){state.status="load-error";return;}
w.DATAFLUX_RUM.init(RUM_CONFIG);state.status="initialized";
if(START_REPLAY){
try{w.DATAFLUX_RUM.startSessionReplayRecording();state.replayStatus="requested";}
catch(e){state.replayStatus="start-error";}
}
}catch(e){state.status="init-error";}
})(window);
`))
	}
	u, _ := json.Marshal(sdkURL)
	integrity := ""
	if s.SDK.Source == "cdn" && s.SDK.Integrity {
		digest, _ := hex.DecodeString(info.SHA256)
		value, _ := json.Marshal("sha256-" + base64.StdEncoding.EncodeToString(digest))
		integrity = "script.integrity=" + string(value) + ";script.crossOrigin=\"anonymous\";"
	}
	return []byte(strings.NewReplacer("SDK_URL", string(u), "SDK_INTEGRITY", integrity, "RUM_CONFIG", string(b), "START_REPLAY", fmt.Sprint(s.RUM.startsReplay())).Replace(`(function(w,d){
"use strict";
if(w.__RUMABLE__||w.DATAFLUX_RUM){return;}
var state=w.__RUMABLE__={status:"loading"};
try{
var script=d.createElement("script");script.async=true;script.src=SDK_URL;SDK_INTEGRITY
script.onerror=function(){state.status="load-error";};
script.onload=function(){
try{if(!w.DATAFLUX_RUM||typeof w.DATAFLUX_RUM.init!=="function"){state.status="unavailable";return;}
var config=RUM_CONFIG;
w.DATAFLUX_RUM.init(config);state.status="initialized";
if(START_REPLAY){
try{w.DATAFLUX_RUM.startSessionReplayRecording();state.replayStatus="requested";}
catch(e){state.replayStatus="start-error";}
}
}catch(e){state.status="init-error";}
};
(d.head||d.documentElement).appendChild(script);
}catch(e){state.status="load-error";}
})(window,document);
`))
}

func sdkIntegrity(info SDKInfo) string {
	digest, _ := hex.DecodeString(info.SHA256)
	return "sha256-" + base64.StdEncoding.EncodeToString(digest)
}
