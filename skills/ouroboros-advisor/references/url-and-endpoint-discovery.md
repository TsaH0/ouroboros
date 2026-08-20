# URL, Endpoint & JS Discovery

Goal: surface every URL, path, and endpoint associated with the target — from web archives, crawling, and JavaScript source — since most interesting bugs (IDOR, XSS sinks, exposed APIs, secrets) live on URLs that aren't linked from the homepage.

## waybackurls (Wayback Machine archive)

```bash
go install -v github.com/tomnomnom/waybackurls@latest

cat httpx/live_urls.txt | waybackurls | sort -u > urls/wayback.txt
```

## gau (GetAllUrls — Wayback + Common Crawl + AlienVault OTX + URLScan)

```bash
go install -v github.com/lc/gau/v2/cmd/gau@latest

cat httpx/live_urls.txt | gau --threads 5 --subs | sort -u > urls/gau.txt
```
Broader source coverage than waybackurls alone; the two are complementary, always run both and merge.

## katana (active crawler, ProjectDiscovery)

```bash
go install -v github.com/projectdiscovery/katana/v2/cmd/katana@latest

katana -list httpx/live_urls.txt -jc -kf all -d 3 -o urls/katana.txt
```
`-jc` = also crawl endpoints found inside JS files. `-d 3` = crawl depth. This is *active* traffic against the target (unlike wayback/gau which are passive archive lookups) — respect rate limits: add `-rl 50` (requests/sec) or `-c 10` (concurrency) if the program requires throttling.

## hakrawler (lightweight active crawler, alternative to katana)

```bash
go install -v github.com/hakluke/hakrawler@latest
cat httpx/live_urls.txt | hakrawler -d 2 -u > urls/hakrawler.txt
```

## Merge & filter URLs

```bash
cat urls/wayback.txt urls/gau.txt urls/katana.txt urls/hakrawler.txt \
  | sort -u > urls/all_urls.txt

# pull out URLs with parameters — these are the ones worth fuzzing for XSS/SQLi/SSRF
grep '=' urls/all_urls.txt > urls/params_urls.txt

# pull out JS files specifically
grep -E '\.js($|\?)' urls/all_urls.txt | sort -u > urls/js_files.txt
```

## getJS / subjs (collect JS file URLs from live pages)

```bash
go install -v github.com/003random/getJS@latest
cat httpx/live_urls.txt | getJS --complete >> urls/js_files.txt
sort -u urls/js_files.txt -o urls/js_files.txt
```

## LinkFinder (extract endpoints/paths from inside JS source — where API routes & secrets hide)

```bash
git clone https://github.com/GerbenJavado/LinkFinder.git
cd LinkFinder && pip install -r requirements.txt --break-system-packages && python3 setup.py install

# per file
python3 linkfinder.py -i https://app.example.com/static/main.js -o cli

# bulk against the collected JS list
while read -r js; do python3 linkfinder.py -i "$js" -o cli; done < urls/js_files.txt \
  | sort -u > urls/js_endpoints.txt
```
This regularly turns up unauthenticated internal API paths, admin panels, and occasionally hardcoded keys/tokens — always worth a manual skim of `js_endpoints.txt`, not just automated triage.

## Notes
- Wayback/gau results can include years of stale/dead URLs — cross-check against `httpx` (probe them, keep only `200`/`301`/`403`) before spending fuzzing time on them.
- `urls/params_urls.txt` is the direct input to the vuln-scanning phase (dalfox/XSStrike for XSS, sqlmap for SQLi).
