# App design (Paper)

The iOS screens, exported from the design canvas:
https://claude.ai/artifact/22AYHiHNCjXiRavYdS2Yfj

Each `*.dc.html` file is one screen (390 × 844, iPhone). They are a visual
spec, not code to ship: the iOS app is SwiftUI, so rebuild each screen
natively. The files load `./support.js` from the canvas, so they won't render
when opened directly; read them as markup, or open the canvas link to see them.
Sample data (names, spots, times) lives in each file's `renderVals()`.

## Style

- Font: Arial / system sans
- Background `#F6F5F1`, text `#16181A`, secondary text `#3C4043` / `#5F6368`
- Borders `#D6D3CB`, dividers `#E4E1D9`, cards `#FFFFFF`
- Accent (studying now, toggles) `#1F7A4D`; badge `#C2410C`; destructive `#B42318`
- Primary button: dark pill, 56 px tall, radius 28; secondary: outlined pill
- Touch targets at least 44 px; long text truncates with an ellipsis

## Screens

| File | Screen | Tickets |
|---|---|---|
| `SignIn.dc.html` | Sign in with Apple | #16, #18 |
| `Setup.dc.html` | Set up profile (name, username, school, major) | #34, #35 |
| `Main.dc.html` | Home: friends studying now and later today | #26, #39, #45 |
| `EmptyHome.dc.html` | Home when no friends are studying | #26 |
| `Studying.dc.html` | Home while you're in a session | #41 |
| `StartSheet.dc.html` | Start session: spot, times, who can see it | #24, #42, #44, #45 |
| `MySession.dc.html` | Your session (edit spot, end time, audience; end) | #25, #41, #46 |
| `Session.dc.html` | A friend's session (join / leave) | #39, #41 |
| `Friends.dc.html` | Friend center: search, requests, friends | #21, #22, #23 |
| `FriendProfile.dc.html` | A friend's profile (remove, block) | #22, #36, #47 |
| `Profile.dc.html` | Your profile: groups, do not disturb, sign out, delete | #19, #20, #38 |
| `EditProfile.dc.html` | Edit profile | #19 |
| `NewGroup.dc.html` | New / edit group | #38 |
| `Blocked.dc.html` | Blocked people | #36 |

## Navigation

- Sign in → Set up profile → Home
- Home: profile button → Your profile; friends button → Friend center;
  Start studying → Start session → Home (you're studying)
- Tap a friend's row → Friend's session → their name → Friend's profile
- Tap your own session card → Your session
- Start session: New group / Edit group → New group
- Your profile → Edit profile, Blocked people, a group → New group
