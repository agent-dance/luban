# LUBAN Code

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [Deutsch](README.de.md)

LUBAN Code ist ein in Go geschriebener Coding-Agent für lange Arbeiten an Repositories. Er lässt das ursprüngliche Sitzungsprotokoll unangetastet und verkleinert nur die Sicht des Modells. Eine andere Proxy-URL ändert außerdem nicht stillschweigend das native Protokoll eines Providers.

> Derzeit gibt es eine Quellcode-Vorschau in Version `v0.1.0`. Binärdateien und Installationen über Paketmanager sind noch nicht veröffentlicht. Bitte aus dem Quellcode bauen.

![Die LUBAN-Code-TUI läuft mit dem OpenAI-Modell gpt-5-6](docs/assets/screenshots/luban-tui.png)

_Die echte TUI, aufgenommen am 12. August 2026 mit dem aktuellen Quellcode-Build unter Windows. API key und endpoint-Adresse sind nicht zu sehen._

## Was LUBAN anders macht

### Die Sicht des Providers wird kleiner, die Belege bleiben erhalten

Lange Sitzungen führen oft zu einer schlechten Wahl: Alte Werkzeugausgaben bleiben vollständig im Prompt, oder eine schwer prüfbare Zusammenfassung ersetzt die Historie. LUBAN bewahrt das ursprüngliche transcript auf. Innerhalb eines eng begrenzten und geprüften Produktivbereichs ersetzt es nur ältere `Inspect`-Ergebnisse in der Provider-Sicht durch eine deterministische Projektion. Pfade, Zeilenbereiche, Seitenangaben, digests und proofs bleiben erhalten.

Vor jeder Projektion schätzt LUBAN die gesamte Anfrage. Die Projektion wird nur zugelassen, wenn die Token-Ersparnis auch nach Cold-Cache- und Wiederherstellungskosten trägt. Ein unbekannter Preis, unvollständige usage-Daten, fehlerhafte Belege oder eine zu kleine Ersparnis führen zur Ablehnung. Fehlerhafte Projektionen werden zurückgenommen. Nach drei aufeinanderfolgenden Auffälligkeiten greift ein Circuit Breaker für die Sitzung.

Der produktive Umfang ist absichtlich klein: `openai/gpt-5.6-sol*` und `deepseek/deepseek-v4-flash*`, ausschließlich für `Inspect`. Der [Entwurf](docs/design/progressive-context-compaction.md) und die [Aufzeichnungen des gepaarten 80k-Laufs](benchmark-results/progressive-context-compaction-v7-80k-2026-08-10/README.md) beschreiben Mechanik und Grenzen.

### Ein Proxy ändert den Weg, nicht den Provider

`BaseURL` legt das Transportziel fest. Auch mit einer eigenen URL behalten OpenAI, DeepSeek, Anthropic, Vertex und Bedrock ihre native Authentifizierung, Cache-Steuerung, Responses- oder Chat-Semantik und ihre providerspezifischen Felder. LUBAN stuft sie nicht unbemerkt zu einem allgemeinen OpenAI-compatible Dialekt herab.

Die automatische Aushandlung von Responses zu Chat gibt es nur für einen ausdrücklich gewählten compatible Provider. Die aktuelle Implementierung deutet `404`, `405` und `501` als fehlenden endpoint. Probleme bei Authentifizierung, Rate-Limits oder schema werden als Fehler zurückgegeben und lösen keinen Protokoll-fallback aus.

### Kleiner Werkzeugkern, klare Betriebsgrenzen

In der standardmäßigen Produktionskonfiguration sieht das Modell drei Coding-Werkzeuge: `Inspect`, `ApplyPatch` und `Run`. Wird der shadow-Pfad für `ContextUpdate` aktiviert, kommt dieses interne Werkzeug hinzu. Darum herum liegen fortsetzbare Sitzungen, parallele Subagenten, optionale Git worktrees, Berechtigungsabfragen, Lifecycle hooks, MCP-Verbindungen und eine NDJSON/Go-SDK-Grenze. Ein Subagent erhält beim Start eine unveränderliche Momentaufnahme seiner Berechtigungen. Spätere Freigaben in der übergeordneten Sitzung erweitern seine Rechte nicht.

Die TUI zeigt Kontext, Cache, Kosten, Komprimierung und die Aktivität von Subagenten. `--screen-reader` bietet eine fortlaufende Ausgabe ohne Cursorsteuerung, Mauserfassung oder Animation. Die Laufzeitoberfläche ist auf Englisch, vereinfachtem Chinesisch, Deutsch, Japanisch, Koreanisch und Russisch verfügbar. Umschalten lässt sie sich mit `Ctrl+L` oder `/language`.

## Messwerte mit nachvollziehbarer Herkunft

Im eingefrorenen lokalen Vergleich mit 15 Aufgaben benötigten die ausgewählten LUBAN-Läufe weniger Zeit, Token und Modellaufrufe als die ausgewählten Codex-Läufe.

| Beobachtete Summe | LUBAN | Codex | Differenz |
| --- | ---: | ---: | ---: |
| Laufzeit | 4.020,6 s | 5.644,5 s | -28,8 % |
| Token | 6.857.490 | 17.889.019 | -61,7 % |
| LLM-Aufrufe | 245 | 354 | -30,8 % |
| Patch erzeugt | 15/15 | 15/15 | gleich |

Das ist eine eingefrorene lokale Stichprobe, keine allgemeine Überlegenheitsaussage. Nur für die ursprünglichen fünf Aufgaben lagen offizielle grader-Ergebnisse vor; beide Agenten lösten 3/5. Die weiteren zehn Aufgaben wurden nicht bewertet. Die Läufe wurden nach der Optimierung ausgewählt, und es gab keinen festen seed. Vor einer weitergehenden Schlussfolgerung sollten der [vollständige HTML-Bericht](benchmark-results/agentic-2026-07-27/representative15-report.html), die [ausgewählten maschinenlesbaren Daten](benchmark-results/agentic-2026-07-27/raw/candidates/selected-15task-20260731.json) und das [Testprotokoll](benchmark/agentic/README.md) gelesen werden.

Auch der Versuch zur schrittweisen Komprimierung ist begrenzt. In einem gepaarten 80k-Lauf war das Ergebnis des eingefrorenen evaluators mit `2/2 + 455/455` gleich. Die total token sanken von `1.362.070` auf `444.419`, die geschätzten Kosten von `$5.207999` auf `$1.004185`. Da kein fester seed verfügbar war, trennten sich die beiden Traces schon vor der ersten Projektion. Die Angaben sind Messwerte zweier realer Traces und Kostenschätzungen auf Grundlage fester Tarife, kein kausaler Durchschnitt der Projektionswirkung.

## Aus dem Quellcode bauen

Benötigt werden Git und die in [`go.mod`](go.mod) angegebene Go-Version, derzeit `1.26.1`. Shell-form-Schritte von `Run` rufen Bash auf. Unter Windows muss daher Git Bash, WSL Bash oder eine andere `bash`-Datei im `PATH` liegen.

macOS oder Linux:

```sh
git clone https://github.com/agent-dance/luban.git
cd luban
go build -o luban-code ./cmd/luban-code
./luban-code --version
```

Windows PowerShell:

```powershell
git clone https://github.com/agent-dance/luban.git
Set-Location luban
go build -o .\luban-code.exe .\cmd\luban-code
.\luban-code.exe --version
```

Das module enthält derzeit ein lokales `replace`. Deshalb wird `go install github.com/agent-dance/luban/cmd/luban-code@latest` nicht unterstützt.

## Provider verbinden und starten

Zugangsdaten lassen sich über Umgebungsvariablen konfigurieren. Sind mehrere Zugangsdaten vorhanden, sollte der Provider ausdrücklich gewählt werden.

```sh
export PROVIDER=openai
export OPENAI_API_KEY="..."
./luban-code
```

```powershell
$env:PROVIDER = "openai"
$env:OPENAI_API_KEY = "..."
.\luban-code.exe
```

DeepSeek verwendet `PROVIDER=deepseek` und `DEEPSEEK_API_KEY` und ist zugleich der Standard-Provider. Ollama nutzt standardmäßig `http://localhost:11434/v1` und das Modell `llama3.1`. Alternativ kann man die TUI starten und mit `Alt+P` Provider, Modell und eine verfügbare Anmeldemethode wählen.

Ein einzelner Lauf ohne TUI:

```sh
./luban-code -p "Prüfe dieses Repository und nenne das größte Risiko"
```

![Ein verifizierter Einzellauf von LUBAN Code v0.1.0 mit der Antwort LUBAN READY](docs/assets/screenshots/luban-live-run.png)

_Der zweite Befehl hat eine echte Anfrage über den lokal konfigurierten OpenAI endpoint gesendet und endete mit Exit-Code 0. Der lokale Prompt-Pfad im Bild wurde unkenntlich gemacht. Das belegt diesen Lauf, nicht die Kompatibilität eines Providers oder die allgemeine Leistung._

In der TUI legt `/init` bei Bedarf `LUBAN.md` und Projekteinstellungen an, ohne vorhandene Dateien zu überschreiben. Zugangsdaten richtet der Befehl nicht ein.

## Grenzen, die vor dem Einsatz bekannt sein sollten

- Das OS sandbox unter Linux benötigt Bubblewrap; macOS verwendet `sandbox-exec`. Windows hat derzeit kein OS sandbox backend. Ohne geprüftes backend bricht `--force-sandbox-tools` den Lauf ab.
- Agent Teams ist eine experimentelle Opt-in-Funktion. Parallele Subagenten und worktree-Isolation sind kein entferntes, verteiltes swarm.
- Registrierte Provider und Protokolltests sind keine Zertifizierung jedes Modells oder Drittanbieter-gateways.
- Lokale Zugangsdaten liegen als Klartext-JSON vor. Unix-ähnliche Systeme schreiben sie mit Modus `0600`; unter Windows gibt es derzeit keine gleichwertige ACL-Garantie. Es handelt sich weder um einen verschlüsselten vault noch um einen OS keychain.
- Node.js wird nur für Node-basierte MCP server benötigt, nicht für die CLI selbst.
- Im Wurzelverzeichnis fehlt noch eine Lizenz. Bis der Owner eine veröffentlicht, gilt das gewöhnliche Urheberrecht.

## Nachweise im Repository

- [Entwurf zur schrittweisen Kontextkomprimierung](docs/design/progressive-context-compaction.md)
- [Bericht zum Rollout der schrittweisen Komprimierung](docs/reports/progressive-context-compaction-rollout-2026-08-11.md)
- [Benchmarkbericht zu 15 Aufgaben](benchmark-results/agentic-2026-07-27/representative15-report.html)
- [Protokoll des Agentic-Benchmarks](benchmark/agentic/README.md)

Für Beiträge gilt [CONTRIBUTING.md](CONTRIBUTING.md), für Sicherheitsmeldungen [SECURITY.md](SECURITY.md).
Die drei Redaktions- und Laufprüfungen in fünf Sprachen stehen im [README-Release-Review](docs/release/readme-review-2026-08-12.md).

Sicherheitsrelevante Funde gehören in GitHubs [private Schwachstellenmeldung](https://github.com/agent-dance/luban/security/advisories/new), nicht in ein öffentliches issue. Welche Angaben benötigt werden, steht in [SECURITY.md](SECURITY.md).
