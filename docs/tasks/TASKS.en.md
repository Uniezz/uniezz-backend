# Uniezz — Backend tasks

Go API. No Supabase. Files go to AWS S3. Web and mobile call this API only.

Estimates: **0.5d or 1d**. Sprint = two weeks, about 10d.

---

## Sprint 1 — Students can sign in

**Goal:** A UMCS student and a student from one Entra university can sign in. We have a health check and a database.

**Not this sprint:** Feed, chat, Meet, Guide, moderator login.

| ID | Task | d | Depends | Done when |
|----|------|---|---------|-----------|
| BE-1.1 | Start the Go API | 1 | — | `/health` works. The old Express app is gone. |
| BE-1.2 | Pick a database and set up migrations | 1 | BE-1.1 | We can create and run one migration. Choice is written down. |
| BE-1.3 | Add user, session, and email-code tables | 1 | BE-1.2 | Tables exist. Empty app still boots. |
| BE-1.4 | Session cookie, “who am I”, and log out | 1 | BE-1.3 | Cookie is HttpOnly. `/auth/me` and `/auth/logout` work. |
| BE-1.5 | How the phone keeps the login | 0.5 | BE-1.4 | We wrote cookie vs token and the API supports it. |
| BE-1.6 | Start login and take the return from the university | 1 | BE-1.4 | `/auth/start` and `/auth/callback` exist for usos, entra, otp. |
| BE-1.7 | Register the USOS app and store keys | 1 | BE-1.6 | Keys are in secrets, not in git. |
| BE-1.8 | Finish USOS sign-in | 1 | BE-1.7 | UMCS student comes back with name and email. PESEL is never saved. |
| BE-1.9 | Register Entra and allow only our universities | 1 | BE-1.6 | Unknown tenant is rejected. |
| BE-1.10 | Block staff on Entra | 1 | BE-1.9 | Staff cannot get a student session (rules from the auth doc). |
| BE-1.11 | Send the email code | 1 | BE-1.6 | Only university emails. Code is stored as a hash. |
| BE-1.12 | Check the email code and rate limits | 0.5 | BE-1.11 | 3 codes per email per hour, 10 per IP, 5 tries, then the code dies. |
| BE-1.13 | One profile shape and a trust badge | 0.5 | BE-1.8, BE-1.10, BE-1.12 | `/auth/me` shows verified, directory, or domain. |

**Sprint total: 11.5d** (auth is tight. If we have no live Entra student yet, prove one tenant in Sprint 2.)

---

## Sprint 2 — Profile, files, Feed

**Goal:** A signed-in student can edit their profile, upload a photo, and post on Feed.

**Not this sprint:** Chat, Meet, Guide.

| ID | Task | d | Depends | Done when |
|----|------|---|---------|-----------|
| BE-2.1 | Read and edit profile text | 1 | BE-1.13 | Name, faculty, year, interests, privacy save. |
| BE-2.2 | Profile photo | 1 | BE-2.1 | Photo URL is on the profile. |
| BE-2.3 | S3 bucket and keys on the server | 1 | BE-1.1 | App has no AWS keys in the client. |
| BE-2.4 | Short-lived upload and download links | 1 | BE-2.3 | Expired link cannot write a file. |
| BE-2.5 | Create a Feed post | 1 | BE-2.1, BE-2.4 | Announcement, event, or question is stored. |
| BE-2.6 | List Feed posts | 1 | BE-2.5 | Newest posts come back. |
| BE-2.7 | Comments on a post | 1 | BE-2.6 | Student can add and read comments. |
| BE-2.8 | Filter Feed | 1 | BE-2.6 | Filter by university, type, and date. |
| BE-2.9 | Report a post, comment, or user | 1 | BE-2.7 | A report row exists for moderators later. |
| BE-2.10 | Delete account and unlink university | 1 | BE-2.1 | Profile and session are gone. |
| BE-2.11 | Try Entra with one live student | 1 | BE-1.10 | We wrote which Graph fields that university fills. |

**Sprint total: 11d** (2.11 can slip if no live account.)

---

## Sprint 3 — Chat

**Goal:** Two students can message. Groups work. New messages show without refresh.

**Not this sprint:** Push notifications to the phone OS. Meet match still unused.

| ID | Task | d | Depends | Done when |
|----|------|---|---------|-----------|
| BE-3.1 | Start a one-to-one chat | 1 | BE-2.1 | Two users share one thread. |
| BE-3.2 | Send and list messages | 1 | BE-3.1 | History loads. Other people cannot read it. |
| BE-3.3 | Create a group chat | 1 | BE-3.1 | Group has a name and members. |
| BE-3.4 | Post in a group | 1 | BE-3.3 | Only members can read and write. |
| BE-3.5 | Live messages | 1 | BE-3.2 | Second client sees a new message without reload. |
| BE-3.6 | Live connection stays up | 1 | BE-3.5 | Reconnect after a drop still works. |
| BE-3.7 | Chat file upload | 1 | BE-2.4, BE-3.2 | Image or file is on the message. |
| BE-3.8 | File size and type limits | 0.5 | BE-3.7 | Bad files are rejected. |
| BE-3.9 | In-app “new message” flag | 1 | BE-3.5 | API can tell the client there is unread. |
| BE-3.10 | Open a chat from a future Meet match | 1 | BE-3.1 | Endpoint exists. Meet will call it in Sprint 5. |

**Sprint total: 9.5d**

---

## Sprint 4 — Study

**Goal:** Students can upload exam files, search them, and rate a course.

**Not this sprint:** Meet, Guide.

| ID | Task | d | Depends | Done when |
|----|------|---|---------|-----------|
| BE-4.1 | Upload an exam file | 1 | BE-2.4 | File is in S3 and listed. |
| BE-4.2 | List exam files | 1 | BE-4.1 | Student sees files for a course. |
| BE-4.3 | Search exam files | 1 | BE-4.2 | Search by title or subject returns hits. |
| BE-4.4 | Course page data | 1 | BE-4.1 | Description and difficulty are stored. |
| BE-4.5 | Rate a course | 1 | BE-4.4 | One rating per student per course. |
| BE-4.6 | Comment on a course | 1 | BE-4.4 | Comments list on the course. |
| BE-4.7 | Filter by university and faculty | 1 | BE-4.2, BE-4.4 | Filter works. |
| BE-4.8 | Filter by semester and subject | 0.5 | BE-4.7 | Filter works. |
| BE-4.9 | Report a file or course | 0.5 | BE-2.9, BE-4.2 | Report shows in the same queue as Feed. |

**Sprint total: 8d**

---

## Sprint 5 — Meet and Guide

**Goal:** Students can swipe. A match opens chat. Staff write Guide places. Students read Guide and review dorms.

**Not this sprint:** Students adding shops. Moderator panel.

| ID | Task | d | Depends | Done when |
|----|------|---|---------|-----------|
| BE-5.1 | List Meet cards | 1 | BE-2.2 | Card has photo, interests, university. |
| BE-5.2 | Like and pass | 1 | BE-5.1 | Choice is saved. Pass does not match. |
| BE-5.3 | Filter Meet | 1 | BE-5.1 | Filter by university, year, intent. |
| BE-5.4 | Match opens one chat | 1 | BE-3.10, BE-5.2 | Two likes create one thread. |
| BE-5.5 | Block Meet without an approved photo | 1 | BE-5.1 | No card list if photo is not ok. |
| BE-5.6 | Staff add or edit a place | 1 | BE-1.3 | Staff can save a shop, hangout, dorm, or store. |
| BE-5.7 | Staff hide or publish a place | 1 | BE-5.6 | Hidden places are not in the student list. |
| BE-5.8 | Students read Guide places | 1 | BE-5.7 | Public list by type. Students cannot create places. |
| BE-5.9 | Dorm reviews | 1 | BE-5.8 | Student can rate and comment on a dorm. |
| BE-5.10 | Map points | 1 | BE-5.8 | Each published place can have a pin. |

**Sprint total: 10d**

---

## Sprint 6 — Moderators

**Goal:** Moderators can sign in (after we write the rules) and handle reports.

Design screen 08 is only a sketch. **Do not build TOTP until BE-6.1 is written.**

| ID | Task | d | Depends | Done when |
|----|------|---|---------|-----------|
| BE-6.1 | Write moderator sign-in in the auth doc | 1 | — | Doc says yes/no to password, TOTP, and idle time. |
| BE-6.2 | Moderator can sign in | 1 | BE-6.1 | Session matches the doc. Students cannot use it. |
| BE-6.3 | Idle timeout if the doc asks for it | 1 | BE-6.2 | Idle session dies. Skip if the doc says no. |
| BE-6.4 | List reports | 1 | BE-2.9 | Moderator sees the queue. |
| BE-6.5 | Approve or reject text | 1 | BE-6.4 | Item leaves the queue. |
| BE-6.6 | Approve or reject media | 1 | BE-6.4, BE-2.4 | Photo or file is kept or removed. |
| BE-6.7 | Warn or suspend a user | 1 | BE-6.5 | Next login shows the block. |
| BE-6.8 | Ban a user | 0.5 | BE-6.7 | Banned user cannot sign in. |
| BE-6.9 | Write an audit log | 1 | BE-6.2 | We can see who took the action. |
| BE-6.10 | Moderators belong to a university | 0.5 | BE-6.2 | Queue can filter by university. |

**Sprint total: 9d**

---

*Document: TASKS backend (EN) · Uniezz · v1.1*
