# OUI database provenance

`data/oui.txt.gz` is a trimmed copy of the IEEE MAC address registry.

| | |
|---|---|
| Source | <https://www.wireshark.org/download/automated/data/manuf> (IEEE MA-L, MA-M and MA-S, republished by Wireshark) |
| Fetched | 2026-07-26 |
| Records | 57,700 (39,549 x 24-bit, 6,487 x 28-bit, 11,664 x 36-bit) |
| Size | 574 KB gzipped, 1.7 MB decompressed |

## Format

Gzipped UTF-8 text. Lines starting with `#` are the header, which carries the
source URL and fetch date that `Metadata()` reports. Every other line is:

```
<hex prefix>\t<vendor name>
```

The prefix length gives the mask width: 6 hex digits is a 24-bit MA-L block,
7 digits a 28-bit MA-M block, 9 digits a 36-bit MA-S block. Records are sorted,
which is what makes the file compress well. Vendor names are collapsed to single
spaces and truncated to 40 characters, back to the last word boundary, because
the tail of "... Technology Co., Ltd. Shenzhen Branch" costs megabytes and tells
a tech nothing.

The committed file predates the word-boundary rule, so one record
(`00:00:00`, the Xerox entry) still ends mid-word. The next refresh fixes it.

## Refreshing

Upstream regenerates the file weekly. From the repository root:

```bash
export PATH=$HOME/.local/go/bin:$HOME/go/bin:$PATH
go run internal/ouidb/gen.go              # downloads a fresh copy
go run internal/ouidb/gen.go -src ./manuf # or use a file you already have
go test ./internal/ouidb/...
```

The generator prints the record counts and file size, and refuses to write a
file with fewer than 10,000 records so an upstream format change cannot silently
empty the database. Update the table above with what it prints.
