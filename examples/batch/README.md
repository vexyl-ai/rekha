# Batch vectorizing with the Rekha API

Everything from the articles on turning a folder of images into SVG, EPS, PDF and DXF through the hosted API at app.rekhasvg.com, or the same API from this repository's server on your own machine.

| File | What it is |
|---|---|
| `REKHA-API.md` | The API in nine lines. Paste it into a prompt, or drop it in the working folder as a facts file for Claude Code. |
| `vectorize.py` | The reference script: every PNG, JPG or GIF in a folder into the formats you list, four requests at a time, resumable, Retry-After honoured. Standard library only. |
| `vectorize-claude-code.py` | What Claude Code wrote from the facts file and the prompt below, unedited (59 lines). |
| `vectorize-chatgpt.py` | What a logged-out ChatGPT wrote from the same facts and request, unedited (53 lines). Works; see the article for the three things to fix. |

## Run

```
export REKHA_KEY=rk_...                      # from https://app.rekhasvg.com/keys
python3 vectorize.py photos/ out/ svg eps pdf dxf
```

Develop against the open-source server instead and nothing counts against a plan:

```
docker run --rm -p 8089:8080 ghcr.io/vexyl-ai/rekha
REKHA_API=http://localhost:8089/v1/trace REKHA_KEY=none python3 vectorize.py photos/ out/ svg eps pdf
```

The container writes SVG, PDF and EPS; DXF and the clean-up pass are in the hosted service.

## The prompt given to Claude Code

With `REKHA-API.md` in the folder and `REKHA_KEY` exported:

```
Read REKHA-API.md. Write vectorize.py, Python 3 standard library only, that converts
every PNG and JPG in the folder given as the first argument into the formats given
as the remaining arguments (default svg) using the Rekha API, saving into a
second-argument output folder, four requests at a time, skipping files that already
exist, retrying 429 and 503 with Retry-After, and printing one line per file.
Then run it on photos/ into out/ with svg and eps using
REKHA_API=http://localhost:8089/v1/trace REKHA_KEY=test and show me the output.
Keep the script under 60 lines.
```

## The prompt given to ChatGPT

The contents of `REKHA-API.md` first, then:

```
Write vectorize.py, Python 3 standard library only, that converts every PNG and JPG
in the folder given as the first argument into the formats given as the remaining
arguments (default svg) using this API, saving into the output folder given as the
second argument, four requests at a time, skipping files that already exist,
retrying 429 and 503 after the Retry-After seconds, and printing one line per file
with the path count and time from the headers. Under 60 lines. Give only the
script, no explanation.
```

## Before trusting a script a model wrote

- The key comes from the environment, never a string in the file.
- On 429 it sleeps for Retry-After and retries a bounded number of times.
- The file field is called `image`.
- Existing output is skipped, so a long run can be resumed.

Articles: [AI vectorizer for 1,000 images](https://rekhasvg.com/ai-vectorizer-batch/), [with Claude Code](https://rekhasvg.com/claude-code-vectorize-images/), [with ChatGPT](https://rekhasvg.com/chatgpt-convert-images-to-svg/). Video: https://youtu.be/QqvRiD66Org
