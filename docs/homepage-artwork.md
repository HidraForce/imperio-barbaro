# Homepage artwork

Created with the built-in image generation tool using the user-provided WhatsApp image as a visual reference. The reference's event text, date, and clan tag are not used as current announcements.

## Final prompt

Use case: stylized-concept. Asset: premium wide cinematic hero illustration for Brazilian Clash of Clans community website Império Bárbaro. Input image role: STYLE REFERENCE only, not an event to advertise. Create a new 3:2 landscape composition inspired by the supplied dramatic stylized 3D fantasy artwork: two powerful blond barbarian kings in gold crowns and red leather armor stand as ALLIES defending their stone fortress, one on either side of a pair of crossed steel swords and a central red banner with a simple gold shield. Rich worn stone, ember particles, warm torchlight, crimson fabric with gold trim, stormy blue-black sky, dramatic yet legible silhouettes, polished game key art. Framing medium-wide, both kings and swords fully visible in the central 75 percent so usable on mobile; distant castle battlements behind them. Heroic, united mood, no gore. No text, no lettering, no numbers, no logos, no watermarks, no event schedule. Keep the flame detail concentrated around edges and base, avoid overwhelming the faces. This is artwork only, not a website mockup.

## Web assets

- `web/public/imperio-hero-768.webp`: 768 × 512, 106,542 bytes.
- `web/public/imperio-hero-1536.webp`: 1536 × 1024, 280,830 bytes.

The generated original was converted to WebP at quality 80. The homepage selects an appropriate version with `srcset` and `sizes`, sets intrinsic dimensions to avoid layout shifts, and prioritizes this above-the-fold image. The rules need no JavaScript. The leadership guide uses the public leadership endpoint; copy-link buttons and rule deep links also use a small local script. No external fonts, social embeds, or private API calls are used. Original rule PDFs are downloaded only when the user follows their links.

## Content sources

The group rules, leadership names and roles, Instagram URL, and WhatsApp channel URL were supplied by the user. Clan-specific rules come from the supplied principal and secondary clan PDFs, copied unchanged as `web/public/regras-principal.pdf` and `web/public/regras-secundario.pdf`. Their requirements are kept separate, including different inactivity limits and mandatory versus recommended participation. The homepage preserves the numeric targets stated by those documents.
