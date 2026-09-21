# parity fixtures

`parity_cases.json` is the shared rule/window matrix used by
`parity_test.go`. `parity_golden.json` is the exact occurrence list
produced for it by the pre-refactor implementation (commit ec8e856).

To regenerate the golden from a code snapshot, copy the non-test
package sources into a temp module with a `main` that loads the cases,
runs `Expand`, and writes the golden:

```sh
rm -rf /tmp/paritygen && mkdir -p /tmp/paritygen/rrule
cp rrule/*.go /tmp/paritygen/rrule/ && rm /tmp/paritygen/rrule/*_test.go
# add /tmp/paritygen/go.mod ("module paritygen") and a main.go that
# decodes parity_cases.json, builds each Schedule, and encodes
# {"golden":[...]} with {name,error,occs:[{wall,instant,zone,floating,allday}]}
cd /tmp/paritygen && go run . path/to/parity_cases.json > rrule/testdata/parity_golden.json
```
