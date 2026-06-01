package main

import (
	"fmt"
	"net/http"
)

func serveLandingPage(w http.ResponseWriter, _ *http.Request, cfg *AppConfig) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, landingPageHTML, cfg.ServerHost, cfg.Port)
}

const landingPageHTML = `<!DOCTYPE html>
<html lang="de">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Liedanzeige</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: "Segoe UI", system-ui, -apple-system, sans-serif;
      background: #fafafa;
      color: #18181b;
      min-height: 100svh;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      padding: 2rem 1rem;
      gap: 2rem;
    }
    header { text-align: center; }
    header h1 {
      font-size: 1.5rem;
      font-weight: 700;
      letter-spacing: -0.02em;
    }
    header p {
      font-size: 0.8125rem;
      color: #71717a;
      font-family: monospace;
      margin-top: 0.375rem;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      gap: 0.75rem;
      width: 100%%;
      max-width: 420px;
    }
    .section-label {
      grid-column: 1 / -1;
      font-size: 0.625rem;
      font-weight: 700;
      letter-spacing: 0.1em;
      text-transform: uppercase;
      color: #a1a1aa;
      padding-top: 0.25rem;
    }
    a.card {
      display: flex;
      flex-direction: column;
      gap: 0.2rem;
      padding: 1rem 1.125rem;
      border: 1px solid #e4e4e7;
      border-radius: 0.75rem;
      background: #fff;
      text-decoration: none;
      color: inherit;
      transition: border-color 0.12s, box-shadow 0.12s;
    }
    a.card:hover {
      border-color: #a1a1aa;
      box-shadow: 0 2px 8px rgba(0,0,0,0.06);
    }
    a.card:active { background: #f4f4f5; }
    .card-tag {
      font-size: 0.625rem;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: #71717a;
    }
    .card-title {
      font-size: 0.9375rem;
      font-weight: 600;
    }
    .card-url {
      font-size: 0.6875rem;
      font-family: monospace;
      color: #a1a1aa;
      margin-top: 0.125rem;
    }
  </style>
</head>
<body>
  <header>
    <h1>Liedanzeige</h1>
    <p>%s:%d</p>
  </header>

  <div class="grid">
    <span class="section-label">Steuerung</span>
    <a href="/steuerung/lied" class="card">
      <span class="card-tag">Operator</span>
      <span class="card-title">Lied</span>
      <span class="card-url">/steuerung/lied</span>
    </a>
    <a href="/steuerung/chor" class="card">
      <span class="card-tag">Operator</span>
      <span class="card-title">Chor</span>
      <span class="card-url">/steuerung/chor</span>
    </a>

    <span class="section-label">Anzeige</span>
    <a href="/lied" target="_blank" class="card">
      <span class="card-tag">Display</span>
      <span class="card-title">Liedanzeige</span>
      <span class="card-url">/lied</span>
    </a>
    <a href="/chor" target="_blank" class="card">
      <span class="card-tag">Display</span>
      <span class="card-title">Choranzeige</span>
      <span class="card-url">/chor</span>
    </a>
  </div>
</body>
</html>`
