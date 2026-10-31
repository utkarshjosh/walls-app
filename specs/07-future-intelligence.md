# Future intelligence: two independent additions

Deferred product direction, not an MVP dependency or a choice of model/provider/library.

## Taste learning

Goal: better discovery as users save, like, hide, or explicitly dislike images. Candidate approaches include embeddings, nearest-neighbor retrieval (KNN), and clustering by subject/style. Compare them with the simple tag baseline before choosing infrastructure.

Saves/likes are positive signals; explicit hides/dislikes are negative. Removing an image from one folder, pausing rotation, or pressing Next is ambiguous and must not automatically mean dislike. Behavioral learning is opt-in, resettable, and optional. Do not infer sensitive personal attributes.

Keep public image similarity separate from private preference data. Private-upload analysis needs informed permission; do not send files to an external model or expose them to other users without consent. Define minimal retention and deletion when implementing.

Start with “More like this” or a better discovery feed. Do not silently change selected folders. A later “keep this collection fresh” option may explicitly authorize ongoing curation with visible scope and an off switch.

Evaluate useful saves, hides, diversity, repetition, and user preference trials; click volume alone is not success. Select storage/model infrastructure only when this phase is scheduled.

## Prompt to curation

Examples: “Make a calm mountain collection with no text,” or “Use aviation and landscapes for my desktop.”

This can work from tags/search and authorized assets without a learned profile. Translate language into existing operations: create a collection, add permitted images, connect folders, or select a frequency preset. A playlist/manifest can be an internal result, never a required new scheduling language.

Flow: request → grounded visual sample and brief proposed changes → Apply → Undo. Show what would change in connected folders, rather than a long future timetable. Do not invent unavailable images, publish private files, delete existing collections, buy content, or redistribute unapproved content as side effects.

Requests beyond supported controls should receive an explanation and the closest supported result, not a hidden elaborate schedule. Use the same authorized APIs as the UI; define bounded execution, recovery, and an undo record when implemented.

## Boundaries

The tracks can combine later, using consented taste signals to improve prompted collections. Both stay secondary to browsing/collecting. No chat-first redesign, AI onboarding dependency, or user-facing KNN/model controls.

First make rotation from connected folders effortless. Then improve how users find and curate those folders.
