# Parameter & Content Discovery

Goal: find hidden GET/POST parameters and hidden directories/files that aren't linked anywhere and won't show up in crawling or archive data.

## arjun (hidden parameter discovery — HTTP Parameter Pollution / hidden param fuzzing)

```bash
pip install arjun --break-system-packages

arjun -u https://app.$TARGET/api/search -oT params/arjun_search.txt
# or bulk against many endpoints
arjun -i httpx/live_urls.txt -oT params/arjun_bulk.txt -t 10
```
Finds params like `debug`, `admin`, `redirect`, `format` that aren't referenced anywhere in the front-end — common source of IDOR/SSRF/auth-bypass bugs.

## paramspider (mines params from archives, complements arjun's active fuzzing)

```bash
git clone https://github.com/devanshbatham/ParamSpider && cd ParamSpider
pip install . --break-system-packages

paramspider -d $TARGET --output params/paramspider.txt
```

## ffuf (fast fuzzer — directories, files, params, vhosts, all-purpose)

```bash
go install -v github.com/ffuf/ffuf/v2@latest

# directory/file brute force
ffuf -u https://app.$TARGET/FUZZ -w /usr/share/seclists/Discovery/Web-Content/raft-medium-directories.txt \
  -mc 200,301,302,403 -o content/ffuf_dirs.json -of json -t 40

# parameter fuzzing on a known endpoint
ffuf -u "https://app.$TARGET/api/user?FUZZ=1" -w seclists/Discovery/Web-Content/burp-parameter-names.txt \
  -mc 200 -fs <baseline_size>
```
Needs a wordlist — [SecLists](https://github.com/danielmiessler/SecLists) is the standard reference set (`raft-*`, `common.txt`, `burp-parameter-names.txt`, etc.).

## gobuster (simpler alternative to ffuf for straightforward dir/DNS brute force)

```bash
go install -v github.com/OJ/gobuster/v3@latest

gobuster dir -u https://app.$TARGET -w /usr/share/seclists/Discovery/Web-Content/common.txt \
  -o content/gobuster.txt -t 30
```

## feroxbuster (Rust, recursive content discovery, fast on large wordlists)

```bash
cargo install feroxbuster

feroxbuster -u https://app.$TARGET -w seclists/Discovery/Web-Content/raft-medium-directories.txt \
  -o content/feroxbuster.txt --depth 3
```

## Notes
- Always set a **baseline** (`-fs <size>` in ffuf, or note the 404 page's content-length) before fuzzing — many apps return `200` for everything with a "not found" body, which silently ruins raw status-code filtering.
- Throttle (`-t`) according to program rules; directory/param fuzzing is the recon phase most likely to trip a program's rate-limit or WAF ban.
- Feed anything interesting found here (new endpoints, params) back into `vuln-scanning.md` for targeted testing.
