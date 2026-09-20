# Uniezz — Product plan

What we build, in which order, and when it is done. Four teams work at the same time: backend, web, mobile, testing.

Sources: [product](../product-description/PRODUCT_DESCRIPTION.en.md) · [auth](../auth/AUTHENTICATION.en.md) · [design](../design/README.md)

## v1 goal

A Lublin student can sign in as a real student, then use Feed, Chat, Study, Meet, Guide, and a moderator can review reports.

## Launch bar

We go public when **UMCS (USOS)** and **at least one other university (Entra)** both work.

- Email code (OTP) is a backup, not the launch story.
- The other Entra universities can come after launch.
- We do **not** wait for all five on day one.

## Not in v1

- Moodle login
- Supabase
- Students adding Guide places (staff write shops, hangouts, dorms, map)
- Database engine and hosting are still open — pick them in Sprint 1, do not hide the choice

## How to read the tasks

| Rule | Meaning |
|------|---------|
| Sprint | Two weeks. All four teams share the same sprint number. |
| Estimate | 0.5d or 1d only. One person on that team. About **10d** per sprint. |
| Depends | Finish this ID first. Web and mobile only talk to the Go API. |
| Done when | Short check. If it is not true, the task is not done. |

Locales: `TASKS.en.md`, `TASKS.uk.md`, `TASKS.ru.md`, `TASKS.pl.md`. IDs match in every language.

## Sprint outcomes

| Sprint | People can… |
|--------|-------------|
| 1 | Sign in (UMCS and one Entra university). See the app shell. |
| 2 | Edit profile, post on Feed, upload photos. |
| 3 | Send chat messages, including groups. |
| 4 | Find exams and course reviews. |
| 5 | Swipe on Meet (if photo is ok). Read the Guide. Review a dorm. |
| 6 | Moderators sign in (after we write the auth rules) and handle reports. |

## Streams

| Stream | File |
|--------|------|
| Backend | [backend/TASKS.en.md](backend/TASKS.en.md) |
| Web | [frontend/TASKS.en.md](frontend/TASKS.en.md) |
| Mobile | [mobile/TASKS.en.md](mobile/TASKS.en.md) |
| Testing | [testing/TASKS.en.md](testing/TASKS.en.md) |
