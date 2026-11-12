package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

//go:embed frontend/*
var embeddedFrontend embed.FS

func main() {
	portFlag := flag.Int("port", 8420, "Local port for the Wails/WebView Anubis GUI")
	headlessCheck := flag.Bool("verify", false, "Verify GUI state and exit cleanly")
	noBrowser := flag.Bool("no-browser", false, "Start GUI server without opening native window")
	flag.Parse()

	cwd, _ := os.Getwd()
	app, err := NewApp(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Anubis GUI Init Error: %v\n", err)
		os.Exit(1)
	}

	if *headlessCheck {
		state := app.GetState()
		b, _ := json.MarshalIndent(state, "", "  ")
		fmt.Println(string(b))
		return
	}

	subFS, _ := fs.Sub(embeddedFrontend, "frontend")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(app.GetState())
	})

	mux.HandleFunc("/api/query", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Prompt string `json:"prompt"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		reply, verdict, recall, _ := app.Engine.Query(context.Background(), body.Prompt)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"reply":   reply,
			"verdict": verdict,
			"recall":  recall,
		})
	})

	mux.HandleFunc("/api/new", func(w http.ResponseWriter, r *http.Request) {
		sessID := app.Engine.NewSession()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session_id": sessID,
			"message":    fmt.Sprintf("🆕 Volledig nieuw gesprek gestart (%s). Vorige context gearchiveerd in .synapse vault.", sessID),
			"state":      app.GetState(),
		})
	})

	mux.HandleFunc("/api/renew", func(w http.ResponseWriter, r *http.Request) {
		sessID, turns := app.Engine.RenewSession()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session_id": sessID,
			"turns":      turns,
			"message":    fmt.Sprintf("🔄 [/renew] Bicameral Bridge & .synapse Fast-Weights vernieuwd voor sessie %s (%d berichten behouden).", sessID, turns),
			"state":      app.GetState(),
		})
	})

	mux.HandleFunc("/api/retry", func(w http.ResponseWriter, r *http.Request) {
		reply, verdict, recall, _ := app.Engine.RetryLast(context.Background())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"reply":   reply,
			"verdict": verdict,
			"recall":  recall,
		})
	})

	mux.HandleFunc("/api/weigh", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Command string `json:"command"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		v := app.Engine.Scales.WeighHeart("gui_weigh", body.Command)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	})

	mux.HandleFunc("/api/model", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model            string `json:"model"`
			ContinuePrevious *bool  `json:"continue_previous"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		cont := true
		if body.ContinuePrevious != nil {
			cont = *body.ContinuePrevious
		}
		msg := app.Engine.SwitchModelWithMode(body.Model, cont)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message":           msg,
			"continue_previous": cont,
			"state":             app.GetState(),
		})
	})

	mux.HandleFunc("/api/duat", func(w http.ResponseWriter, r *http.Request) {
		scrollPath, _ := app.Engine.Duat.WriteMorningScroll()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"scroll_path": scrollPath})
	})

	mux.HandleFunc("/api/update", func(w http.ResponseWriter, r *http.Request) {
		out, err := exec.Command("/opt/homebrew/bin/annubis", "update").CombinedOutput()
		msg := string(out)
		if err != nil && msg == "" {
			msg = err.Error()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": msg})
	})

	addr := fmt.Sprintf("127.0.0.1:%d", *portFlag)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to bind GUI port: %v\n", err)
			os.Exit(1)
		}
	}
	url := "http://" + ln.Addr().String()
	fmt.Printf("𓂀 ANUBIS AGENT GUI running at %s\n", url)

	if !*noBrowser && runtime.GOOS == "darwin" {
		_ = exec.Command("open", url).Start()
	}
	_ = http.Serve(ln, mux)
}
