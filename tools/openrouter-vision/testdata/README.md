# Opt-in real-image fixtures for OpenRouter Vision

This directory contains a **downloader and live evaluation script**, not
committed image binaries. Run it locally to obtain a reproducible set of
real photographs; the photos are ignored by Git. This keeps the repo small
and avoids redistributing third-party photos unnecessarily.

The images are from Wikimedia Commons. The photographer, original file,
applicable license and intended test label are recorded here. Always inspect
the source page and any rights of depicted people before uploading images
to an external AI provider. Copyright reuse permission is **not** the same
thing as a person's consent or a provider's data-processing authorization.

## Cases and licenses

| ID | Scenario | Expected output | Source, credit and license |
| --- | --- | --- | --- |
| P1 | Airsoft squad, visible people | `present` | [Airsoft squad](https://commons.wikimedia.org/wiki/File:Airsoft_squad.jpg), Od5563 / BEAR Airsoft, [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) |
| P2 | Players on a field | `present` | [AirsoftRockField](https://commons.wikimedia.org/wiki/File:AirsoftRockField.jpg), Michael Brown (UNHchabo), [CC BY 2.5](https://creativecommons.org/licenses/by/2.5/) (dual-licensed) |
| P3 | Camouflaged airsoft player | `present` | [Airsoft sniper](https://commons.wikimedia.org/wiki/File:Airsoft_sniper.jpg), Kokanee, public domain |
| C1 | Ordinary people walking outdoors | `present` | [People walking in park](https://commons.wikimedia.org/wiki/File:People_walking_in_park_in_front_of_bay_and_Berkeley_shore_(7114041597).jpg), D Coetzee, [CC0](https://creativecommons.org/publicdomain/zero/1.0/) |
| N1 | Forest without people | `absent` | [Forest nature trail](https://commons.wikimedia.org/wiki/File:Forest_nature_trail.jpg), Steve Hillebrand / US Fish and Wildlife Service, public domain |
| N2 | Airsoft scope without a person | `absent` | [Airsoft Bushnell rifle scope](https://commons.wikimedia.org/wiki/File:Airsoft_Bushnell_rifle_scope.JPG), BrokenSphere, [CC BY-SA 3.0](https://creativecommons.org/licenses/by-sa/3.0/) |
| N3 | Mannequin in uniform | Review only | [Singapore Army mannequin](https://commons.wikimedia.org/wiki/File:Mannequin_in_a_Singapore_Army_uniform_on_display_at_the_National_Museum_of_Singapore_-_20140223.jpg), Sgconlaw, [CC BY-SA 3.0](https://creativecommons.org/licenses/by-sa/3.0/) |

N3 is intentionally informational: the coordinator accepts this particular
false positive for the initial Sprint 4 PoC. That does not mean the model
distinguishes mannequins from real people. C1 is the new *casual-clothes*
regression test; results have not been independently verified yet.

## Commands (Mac, Linux)

From `tools/openrouter-vision`, fetch photos into the ignored `testdata/photos`
directory. This performs **downloads only**, not AI API calls:

```bash
python3 testdata/vision_samples.py fetch
```

The script verifies the downloaded JPEG signature and the 2 MiB maximum.
To run selected images, after setting your API key and model:

```bash
export OPENROUTER_MODEL="nvidia/nemotron-3.5-super-vl:free"
python3 testdata/vision_samples.py run --ids P1,C1,N1 --send-images
```

Run the entire suite by omitting `--ids`. Each selected image causes **one
external model request**; there are no automatic retries. The
`--send-images` flag is deliberate consent for the chosen samples; it does
not override third-party rights or provider usage conditions. Some images
could contain identifiable people, so **do not run uploads** unless you
have checked that the chosen samples are allowed by your provider.

Expected statuses are test hypotheses, not guaranteed performance. The
script shows actual model status and a normalized box; bbox **placement**
must still be inspected manually on the photo. A failed request is reported
as an error and is never counted as a successful `absent`.

## CI

No API key is used in GitHub Actions; ordinary Go tests rely only on
`httptest` and a generated tiny PNG. Network downloads and paid/free model
requests are deliberately opt-in and do **not** run in CI. If the Wikimedia
CDN is temporarily unavailable, this does not make the repository's CI red.
