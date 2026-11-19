# Committed OpenRouter Vision photographs

The 7 images in photos/ are real, versioned photographs from
Wikimedia Commons, copied from public GitHub mirrors that preserve the
source credit. There is no download step. expected.json records the
expected class, original photo, license, mirror and exact Git blob SHA-1.
The source JPEGs and downsized WebP copies are below the CLI's 2 MiB limit.

These images were not created by an AI model. The photo rights remain
with their respective owners; the repository's code license does not
replace the individual licenses. The WebP versions were resized and
converted by the source mirror, not by this TAC OS project. Retain this
attribution when redistributing the photos.

| ID | Photo / use | Credit | Source and license |
| --- | --- | --- | --- |
| P1 | Airsoft squad; people present | Od5563 / BEAR Airsoft | [Original](https://commons.wikimedia.org/wiki/File:Airsoft_squad.jpg), [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) |
| P2 | Airsoft team; people present | Mamusque | [Original](https://commons.wikimedia.org/wiki/File:ASD_Decima_Airborne_Softair_Team_-_Pisa_-_In_Action.jpg), [CC BY-SA 3.0](https://creativecommons.org/licenses/by-sa/3.0/) |
| P3 | Airsoft shooter; person present | Myneone | [Original](https://commons.wikimedia.org/wiki/File:Airsoft_Designed_Shooter.jpg), [CC BY-SA 3.0](https://creativecommons.org/licenses/by-sa/3.0/) |
| C1 | Ordinary people walking outdoors | D Coetzee | [Original](https://commons.wikimedia.org/wiki/File:People_walking_in_park_in_front_of_bay_and_Berkeley_shore_(7114041597).jpg), [CC0 1.0](https://creativecommons.org/publicdomain/zero/1.0/) |
| N1 | Woodland path; no person | Ann Cook | [Original](https://commons.wikimedia.org/wiki/File:Heading_for_the_fork_in_the_path_-_geograph.org.uk_-_3327285.jpg), [CC BY-SA 2.0](https://creativecommons.org/licenses/by-sa/2.0/) |
| N2 | AA batteries; no person | ajay_suresh | [Original](https://commons.wikimedia.org/wiki/File:AA_Batteries_(51825378856).jpg), [CC BY 2.0](https://creativecommons.org/licenses/by/2.0/) |
| N3 | Suitcases; no person | Acabashi | [Original](https://commons.wikimedia.org/wiki/File:Baggage_cases_luggage_trunks_Epping-Ongar-Railway_North_Weald_Essex_England.jpg), [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) |

**Dataset change:** only P1 is the exact same file used in the earlier
operator-reported OpenRouter test. P2, P3, N1, N2 and N3 now use different
licensed photos (or photos of equivalent cases). C1 is the casual-clothes
case. Do NOT carry old model results over to these new files. In
particular N3 is now luggage, NOT the previously tested mannequin.

## Run after git pull

From tools/openrouter-vision:

    python3 testdata/vision_samples.py list
    python3 testdata/vision_samples.py check

Both commands run entirely offline. The check command verifies that each
committed image is a valid JPEG/WebP of at most 2 MiB and has the exact
Git blob hash recorded in expected.json.

For explicitly permitted uploads to an external provider, set the API
key and a vision-capable model, then:

    export OPENROUTER_MODEL="nvidia/nemotron-3.5-super-vl:free"
    python3 testdata/vision_samples.py run --ids P1,C1,N1 --send-images

Omit --ids to run all seven. One image = one model call, without
retries. Each call has financial/provider consequences depending on
the account and model. The --send-images flag deliberately signals
that the operator accepts external image transmission. It does NOT
override provider restrictions on processing photographs of people.
Confirm those permissions and any applicable consent requirements
before running. No photo or API key is sent by the normal GitHub CI.

The present/absent values in expected.json are initial labels for a
person visible, not identification as an opponent or proof of an ArUco ID.
Bounding-box geometry needs a separate human visual review; valid
normalized numbers alone do not prove localization accuracy.
