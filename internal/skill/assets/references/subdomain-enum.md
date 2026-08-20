# Subdomain Enumeration

Goal: build as complete a list as possible of subdomains under the in-scope root domain(s), then dedupe.

## subfinder (passive, fast, first choice)

```bash
# install
export GOPROXY=direct
go install -v github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest

# usage
subfinder -d $TARGET -all -silent -o subdomains/subfinder.txt
```
Passive-only by default (queries sources like crt.sh, VirusTotal, Shodan if API keys configured in `~/.config/subfinder/provider-config.yaml`). Safe rate-wise since it never touches the target directly.

## amass (passive + active, most thorough, slower)

```bash
go install -v github.com/owasp-amass/amass/v4/...@latest

# passive only (safe default for bug bounty scope respect)
amass enum -passive -d $TARGET -o subdomains/amass.txt

# active (brute force + DNS resolution) — only if program allows active recon
amass enum -active -d $TARGET -o subdomains/amass_active.txt
```

## assetfinder (lightweight passive)

```bash
go install -v github.com/tomnomnom/assetfinder@latest
assetfinder --subs-only $TARGET | tee subdomains/assetfinder.txt
```

## findomain (fast, Rust-based)

```bash
# via cargo
cargo install findomain
findomain -t $TARGET -u subdomains/findomain.txt
```

## crt.sh (certificate transparency, no install needed)

```bash
curl -s "https://crt.sh/?q=%25.$TARGET&output=json" \
  | jq -r '.[].name_value' | sed 's/\*\.//g' | sort -u > subdomains/crtsh.txt
```
Good zero-dependency fallback or cross-check source.

## Merge, dedupe, resolve

```bash
cat subdomains/*.txt | sort -u > subdomains/all_raw.txt

# resolve to confirm which actually exist (kills stale/wildcard noise)
go install -v github.com/projectdiscovery/dnsx/v2/cmd/dnsx@latest
cat subdomains/all_raw.txt | dnsx -silent -o subdomains/resolved.txt
```

Feed `subdomains/resolved.txt` into the next phase (`httpx` in `http-probing-and-fingerprinting.md`).

## Notes
- Wildcard DNS can pollute brute-force results with false positives — `dnsx -wd $TARGET` filters wildcard responses.
- Always prefer passive-only enumeration unless the program's scope explicitly allows active brute-forcing; active DNS brute force generates a lot of traffic against the target's DNS infra.
