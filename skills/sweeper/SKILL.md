---
name: sweeper
description: "Parent duty after the builder's commit: run the sweep and store the sweeper and polisher records. Trigger: the role-chain skill points here after the builder."
disable-model-invocation: true
license: MIT
metadata:
  author: labdrian
  version: "1.0"
---

## Triggers

- Sweeper, on the builder's commit (`base` = parent): `labdrian review sweep --feature F --base B --head H` runs T1-T3 (`labdrian --help`); if it fires, launch the sweeper on its candidates, store its report with `labdrian review record --role sweeper`; if the `Risk:` trailer names an item, store each polisher judge's findings with `labdrian review record --role polisher`.
