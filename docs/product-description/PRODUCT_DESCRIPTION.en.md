# Uniezz — Product Description

## Overview

**Uniezz** is a web application for students of Lublin universities. The platform brings together students from five institutions in one space: communication, knowledge sharing, practical information about life in Lublin, and finding new connections.

### Target Universities

- **UMCS** — Maria Curie-Skłodowska University
- **KUL** — John Paul II Catholic University of Lublin
- **Politechnika Lubelska** — Lublin University of Technology
- **Uniwersytet Przyrodniczy** — University of Life Sciences in Lublin
- **WSEI** — Lublin University of Economics and Innovation

---

## Core Modules

### 1. Authentication

Sign-in through university service accounts (likely **Moodle** and equivalent systems at each institution). This ensures that only active students have access to the platform.

- Single sign-on (SSO) via integration with university systems
- Profile linked to a specific university, faculty, and year of study
- Student status verification

### 2. Profile & Connections

- Personal profile: photo, name, university, faculty, year, interests
- Connections list (friends, classmates, acquaintances)
- Privacy settings
- Ability to find students from your own and other Lublin universities

### 3. Feed (Announcements)

A social feed for the student community:

- **Announcements** — buy/sell, roommate search, services
- **Events** — parties, meetups, conferences, sports events
- **Questions** — academic help, everyday issues, advice for newcomers
- **Comments** — discussion under each post
- Filtering by university, category, and date

### 4. Chat (Messenger)

Built-in messenger for private and group communication:

- Direct messages between users
- Group chats (by subject, faculty, dormitory, etc.)
- Notifications for new messages
- Text and media sharing (with moderation)

### 5. Academics & Student Help

A section for academic support:

- **Past exam papers** — archive of assignments, exam questions, and tests
- **Ratings & reviews** — scores and comments about professors and courses
- **Course descriptions** — syllabus, difficulty level, study recommendations
- Search and filtering by university, faculty, semester, and subject
- Ability to leave comments and contribute materials

### 6. Student Guide (Essential Info)

A practical guide to student life in Lublin:

- **Shopping** — where to buy cheaper, store comparisons (Biedronka, Lidl, Carrefour, etc.)
- **Places to hang out** — cafés, bars, parks, cultural spots
- **Dormitories** — descriptions, rent costs, conditions, student reviews
- **Nearby stores** — what to buy where (groceries, stationery, electronics, clothing)
- Map with points of interest marked

### 7. Connections (Tinder-like)

A module for finding friends, socializing, and dating:

- User cards with photo, interests, and university
- Swipe mechanics: like / pass
- Match on mutual interest — chat opens
- Filters: university, year, goals (friends / relationships / socializing)
- Strict profile and photo moderation

---

## Multilingual Support

The application supports four languages:

| Language | Code |
|----------|------|
| Polish | `pl` |
| Ukrainian | `uk` |
| Russian | `ru` |
| English | `en` |

- Language switch in profile settings
- Full interface localization
- User-generated content displayed in the author's language; interface in the selected language

---

## Moderation & Safety

Strict content moderation is a core principle of the platform:

- **Text** — automated and manual review for insults, racism, discrimination, bullying, and negativity
- **Media** — review of uploaded photos and videos
- **Reports** — users can flag violations
- **Sanctions** — warnings, temporary suspension, permanent ban
- **Moderators** — a team of representatives from each university

---

## Value for Students

| Problem | Uniezz Solution |
|---------|-----------------|
| Scattered chats and groups across social media | One platform for all Lublin universities |
| Hard to find exam materials | Exam archive and course reviews |
| Newcomers don't know where to live or shop | Guide with up-to-date information |
| Difficult to meet people outside your university | Connections module across the city |
| Toxic content in open groups | Strict moderation and student verification |

---

## Tech Stack (Planned)

- **Frontend:** React + TypeScript + Vite + TanStack Router (web); Expo / React Native (mobile)
- **Backend:** Go — own API. No Supabase or other BaaS; web and mobile talk only to this API
- **Authentication:** USOS (OAuth 1.0a), Microsoft Entra ID (OIDC), email OTP fallback — see `docs/auth/`
- **Media:** AWS S3 — photos, chat attachments, exam files, and other user uploads
- **Database:** TBD
- **Hosting:** TBD

---

*Document: PRODUCT_DESCRIPTION (EN) · Uniezz · v1.2*
