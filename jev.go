// Jev (TypeSafe System One) client — fast (70~500ms) structured judgments for adapters,
// e.g. "what kind of chat is this / does it need my reply". Optional: without
// TYPESAFE_API_KEY, callers fall back to rules. Docs: https://docs.typesafe.ai/primitives/choice
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const jevEndpoint = "https://api.typesafe.ai/v1/systemone"

func jevAvailable() bool { return os.Getenv("TYPESAFE_API_KEY") != "" }

func jevChoice(instructions string, criteria map[string]string) map[string]any {
	return map[string]any{"type": "choice", "instructions": instructions, "criteria": criteria}
}

func jevScore(instructions string, criteria []string) map[string]any {
	return map[string]any{"type": "score", "instructions": instructions, "criteria": criteria}
}

func jevNoul(instructions string) map[string]any {
	return map[string]any{"type": "noul", "instructions": instructions}
}

// jevAnswer is one answer: choice → Choice, score → Score, noul → Noul (probability).
type jevAnswer struct {
	Choice string  `json:"choice"`
	Score  any     `json:"score"`
	Noul   float64 `json:"noul"`
}

// jevSystemOne sends several questions about one state in a single call (evaluated in parallel).
func jevSystemOne(state any, questions map[string]map[string]any) (map[string]jevAnswer, error) {
	model := env("TYPESAFE_MODEL", "jev-latest")
	body, _ := json.Marshal(map[string]any{"model": model, "state": state, "questions": questions})
	req, _ := http.NewRequest("POST", jevEndpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+os.Getenv("TYPESAFE_API_KEY"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("jev HTTP %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Answers map[string]jevAnswer `json:"answers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("jev: bad response: %v", err)
	}
	return out.Answers, nil
}
