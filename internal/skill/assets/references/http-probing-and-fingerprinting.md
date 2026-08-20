# Live Host Probing, Port Scanning & Tech Fingerprinting

Goal: turn a subdomain list into a list of confirmed-alive HTTP(S) hosts, tagged with status code, title, and tech stack — the input every later phase consumes.

## httpx (the workhorse — always run this before anything else)

```bash
export GOPROXY=direct
go install -v github.com/projectdiscovery/httpx/v2/cmd/httpx@latest

cat subdomains/resolved.txt | httpx -silent \
  -status-code -title -tech-detect -web-server -content-length \
  -o httpx/live.txt
```
Output line looks like: `https://app.example.com [200] [App Login] [nginx] [React]`. This one file drives everything downstream — extract just the URLs with:
```bash
cat httpx/live.txt | awk '{print $1}' > httpx/live_urls.txt
```

Useful extra flags:
- `-mc 200,301,302,403` — filter by status codes worth investigating
- `-fc 404` — drop known-dead responses
- `-threads 50 -rate-limit 150` — respect program rate limits
- `-follow-redirects`
- `-screenshot` (httpx has a lightweight built-in screenshotter via `-ss`, alternative to gowitness for quick triage)

## naabu (fast port scanning)

```bash
go install -v github.com/projectdiscovery/naabu/v2/cmd/naabu@latest

naabu -list subdomains/resolved.txt -top-ports 1000 -o httpx/ports.txt
```
Use only if the program's scope allows port scanning (some bug bounty programs restrict this to web ports only — check rules). Pipe results into `httpx` to probe non-standard HTTP ports:
```bash
naabu -list subdomains/resolved.txt -p 80,443,8080,8443,8000,8888 -silent \
  | httpx -silent -o httpx/nonstandard_ports.txt
```

## whatweb (classic tech fingerprinting)

```bash
sudo apt install -y whatweb   # or: gem install whatweb

whatweb -i httpx/live_urls.txt --log-json=httpx/whatweb.json -a 3
```
`-a 3` = aggression level 3 (more accurate, more requests). Good for CMS/framework/plugin version detection that `httpx -tech-detect` sometimes misses (e.g. WordPress plugin versions).

## Wappalyzer CLI (browser-fingerprint-DB based, complements whatweb)

```bash
npm install -g wappalyzer
wappalyzer https://app.$TARGET --pretty
```

## Notes
- Always dedupe and cross-reference `httpx -tech-detect`, `whatweb`, and Wappalyzer results — no single fingerprinter catches everything, and knowing the exact stack (e.g. "WordPress 6.2 + WooCommerce 8.1") is what lets you pick relevant `nuclei` template tags later.
- If a WAF is detected (`httpx` flags this, or check response headers/`wafw00f`), throttle scan speed and mention it in the recon summary — it changes which vuln-scan techniques will even land (see `vuln-scanning.md`).
