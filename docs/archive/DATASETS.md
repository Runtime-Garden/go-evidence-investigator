# Public datasets for Evidence Investigator

The default dataset for project reproducibility is **SciFact**. The `data/demo` corpus is artificial and is not a research dataset.

| Dataset | Domain | Evidence format | Fit for project | Legal/provenance considerations |
|---|---|---|---|---|
| SciFact | Scientific abstracts | Claims, corpus, document IDs, sentence rationale labels | Recommended first benchmark | Original upstream: claim/evidence annotations CC BY 4.0, abstracts ODC-By 1.0; cite Wadden et al. |
| FEVER | Wikipedia-derived general knowledge | ~185k labeled claims, linked Wikipedia sentence evidence | Second phase for scale and open-domain questions | Wikipedia-specific terms and CC BY-SA 3.0 (or page-specific terms); attribution/share-alike obligations need review |
| AVeriTeC | Real-world fact-checking | Claims, supporting Q&A, web evidence/knowledge store | Third phase for external source diversity and time awareness | AVeriTeC shared-task dataset/baseline reported under CC BY-NC 4.0; third-party page content and URLs require separate attention |

Official links:

- SciFact: https://github.com/allenai/scifact ; https://github.com/allenai/scifact/blob/master/LICENSE.md ; https://github.com/allenai/scifact/blob/master/doc/data.md
- FEVER: https://fever.ai/dataset/fever.html ; https://fever.ai/download/fever/license.html
- AVeriTeC: https://fever.ai/dataset/averitec.html ; https://github.com/MichSchli/AVeriTeC ; shared-task paper https://aclanthology.org/2024.fever-1.1.pdf

## How to use SciFact correctly

1. Download the official archive using `python3 scripts/fetch_scifact.py` and record its SHA-256. In versioned experiment reports, note download date, URL and hash.
2. Start with the full `corpus.jsonl` (all abstracts). **Do not** preselect gold evidence documents based on `claims_dev.jsonl`: that would make recall artificially strong.
3. Use `claims_train.jsonl` for development only. Evaluate retrieval on `claims_dev.jsonl` with the gold evidence map, outside the inference pathway.
4. Preserve `doc_id`, `sentence_index`, sentence quotation, and quote SHA-256 in every report. Distinguish dataset metadata from a real source publisher URL.
5. Calculate document recall@k and later sentence rationale recall@k, verdict macro F1, abstention coverage and citation faithfulness. Keep model API parameters and cost logs in experiment records.
6. Disclose limits: sources are abstracts, cannot reliably capture all context of full papers; many scientific claims involve statistical reasoning; absence of retrieved evidence is not a refutation.

## Dataset adapter contract

Current corpus JSONL example:

```json
{"doc_id": 42, "title": "Synthetic Example", "abstract": ["A sentence of artificial evidence."]}
```

Current annotated claim JSONL example:

```json
{"id": 1, "claim": "An artificial assertion.", "evidence": {"42": [{"label": "SUPPORT", "sentences": [0]}]}}
```

Adapter for FEVER/AVeriTeC would need a separate parser and evaluation adapter; **these datasets are not yet supported by the v0.1 loader**. Do not imply support merely because the datasets are recommended.
