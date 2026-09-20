# Uniezz — Authentication

## Overview

Uniezz verifies that a user is a real, active student before granting access. Five universities, three identity providers, one entry point on the **Go API** for web and mobile. No Supabase.

| University | Provider | Verified data |
|------------|----------|---------------|
| **UMCS** | USOS API (OAuth 1.0a) | Identity, faculty, programme, year |
| **Politechnika Lubelska** | Microsoft Entra ID (OIDC) | Identity, university; faculty if published |
| **Uniwersytet Przyrodniczy** | Microsoft Entra ID (OIDC) | Identity, university; faculty if published |
| **KUL** | Microsoft Entra ID (OIDC) | Identity, university; faculty if published |
| **WSEI** | Microsoft Entra ID (OIDC) | Identity, university; faculty if published |
| *any of the five* | Email OTP | University domain only — fallback |

### Approaches Considered

| Approach | Verdict |
|----------|---------|
| **USOS API (OAuth 1.0a)** | Chosen for UMCS. Official, verified data, self-service registration |
| **Microsoft Entra ID (OIDC)** | Chosen for the other four. All five run Entra tenants |
| **Email OTP on university domain** | Fallback when admin consent blocks the Entra app |
| Moodle Web Services | Rejected. Token issued manually by university IT |
| Scraping the campus login form | Rejected. Handling university passwords is a legal and security liability |

---

## Provider 1 — UMCS via USOS API

UMCS runs **USOS**, the academic management system used by over 50 Polish institutions, and exposes a public **USOS API**. This is the only Lublin university in the official USOS API installations registry.

### Developer Registration

Self-service — no approval process, no contract with the university.

1. Open https://apps.umcs.pl/developers/
2. Submit the application name, an optional homepage URL, and a valid contact email
3. Receive **Consumer Key** and **Consumer Secret**

The Consumer Secret never leaves the backend and is never committed. A revocation form exists at the same address; confirmation is sent to the registered email.

### OAuth 1.0a Flow

USOS API uses OAuth 1.0a (three-legged), not OAuth 2.0. All request signing happens on the backend with HMAC-SHA1.

| Step | URL |
|------|-----|
| Request token | `https://apps.umcs.pl/services/oauth/request_token` |
| Authorize | `https://apps.umcs.pl/services/oauth/authorize` |
| Access token | `https://apps.umcs.pl/services/oauth/access_token` |

1. **Request token** — backend calls `request_token` with the Consumer Key, the requested `scopes`, and an `oauth_callback` pointing at Uniezz
2. **Redirect** — the user is sent to `authorize` and logs in with their UMCS credentials on the university's own page
3. **Consent** — the user approves the scopes; USOS redirects back with `oauth_token` and `oauth_verifier`
4. **Access token** — backend exchanges the verifier for a long-lived access token and secret
5. **Fetch profile** — backend calls the API with the signed token and creates or updates the Uniezz account

The user's UMCS password is entered only on the UMCS page and is never visible to Uniezz.

### Scopes

None of these require administrator approval.

| Scope | Purpose in Uniezz |
|-------|-------------------|
| *(default)* | Basic identity — user ID, first and last name |
| `studies` | Programmes, courses, group lists — the source of faculty, programme, and year |
| `email` | University email address, used as the account identifier |
| `photo` | Profile photo and its visibility preferences |
| `personal` | Date of birth — age gates, birthday features, age filters in Connections |
| `offline_access` | Long-lived token so the profile can be refreshed between sessions |

**On `personal`:** this scope is requested for the date of birth, which Uniezz needs for age gates and for age filters in the Connections module. The same scope also returns PESEL and other identifiers — those are discarded at read time and never written to the database or the logs.

**Deliberately not requested:** `grades`, `payments`, `mobile_numbers`. Uniezz has no use for them and requesting them lowers consent rates.

### Data Retrieved

Read through `services/users/user`, with the `fields` parameter naming each field explicitly (USOS API requires it): user ID, first and last name, university email, date of birth, student programmes, student status, photo URL. Faculty details are resolved from the programme through the faculty and programme endpoints. The exact field set is finalised during integration against https://apps.umcs.pl/developers/api/

---

## Provider 2 — Entra ID for the Other Four

All five universities run Microsoft Entra ID tenants, confirmed against the Microsoft tenant discovery endpoint. For the four without a public academic API, Entra is the strongest available proof: it confirms a live account in the university directory, not just ownership of an email address.

### Tenants

| University | Domains | Tenant ID | Type |
|------------|---------|-----------|------|
| Politechnika Lubelska | `pollub.edu.pl`, `student.pollub.edu.pl` | `dbb41d7a-0043-4ee2-9843-6e4ff66cc9c8` | Managed |
| Uniwersytet Przyrodniczy | `up.lublin.pl`, `student.`, `stud.` | `25a59194-f151-40c5-9e45-365a4d46d7b9` | Managed |
| WSEI | `wsei.lublin.pl`, `student.`, `stud.` | `bab1e1e4-b3e8-49c1-93e4-eaeff72f1f5d` | Managed |
| KUL — students | `student.kul.pl` | `f445c1ee-43fc-42e8-b642-b382d382c3c1` | Managed — **accepted** |
| KUL — staff | `kul.pl` | `7952c9f0-b177-4a4c-ab55-7c2f5ab0808d` | Managed — **rejected** |
| UMCS | `umcs.pl` | `80dbd34a-9b20-490b-ac49-035af103ab2b` | Federated (own SAML IdP) |

KUL runs two separate tenants and is the only university where students are already isolated at the tenant level: the student tenant is accepted, the staff tenant is rejected. UMCS is listed for completeness — it authenticates through USOS instead.

### Setup

Register one **multi-tenant** application in Entra ID, then allow only the tenant IDs marked as accepted above. A user whose `tid` is not on the list is rejected, which keeps the login restricted to Lublin universities.

### Students Only

Uniezz is a student platform — staff accounts must not be able to sign in. The tenant ID alone is not enough to enforce this, because Politechnika Lubelska, Uniwersytet Przyrodniczy, and WSEI keep students and staff in one tenant. Enforcement is layered:

1. **Tenant allowlist** — settles KUL outright, since its students live in their own tenant
2. **UPN domain** — where the university issues student mail on a subdomain (`student.`, `stud.`), only those addresses are accepted
3. **Directory attributes** — `employeeType` and `jobTitle` from Graph are checked; a value indicating staff is rejected
4. **Manual review** — if none of the above yields a decision, the account is created with reduced rights and flagged for review

Steps 2 and 3 depend on how each university provisions its accounts, which cannot be determined without a live account. Until that is confirmed per university, the student/staff split is enforceable with certainty only for KUL and, through `student_status`, for UMCS.

### Guaranteed Claims

Available from the ID token with no extra request:

| Claim | Use |
|-------|-----|
| `oid` | Stable user ID within the tenant — primary key |
| `tid` | Tenant ID — resolves the university with certainty |
| `name` | First and last name |
| `preferred_username` | UPN, normally the university email |
| `email` | Email when published by the administrator; otherwise fall back to the UPN |

### Optional Profile Fields

Requested from Microsoft Graph with the `User.Read` scope, which the user consents to directly — no administrator involved. The default `/me` response does **not** include `department`, so `$select` is required:

```
GET https://graph.microsoft.com/v1.0/me?$select=id,displayName,mail,userPrincipalName,jobTitle,department,companyName,officeLocation,employeeType
```

- `department` — usually the faculty
- `jobTitle`, `employeeType` — sometimes "Student"
- `companyName` — the university name
- `officeLocation` — building or campus
- `GET /me/photo/$value` — profile photo, same scope
- `birthday` — date of birth; present in the schema but in practice almost never populated for students, so treat it as self-declared at onboarding

These fields are populated by the university administrator during provisioning. They may be complete or empty, and each tenant differs. Verifying this requires one live student account per university.

**Never available:** year of study, group number, semester, grades. That data lives in the faculty systems (eHMS, Wirtualny Dziekanat, e-KUL), none of which expose a public API.

---

## Provider 3 — Email OTP Fallback

Used when a university administrator has disabled user consent for third-party multi-tenant applications, in which case the Entra login returns "Need admin approval" instead of a consent screen.

1. The user enters a university email; the domain is checked against the whitelist
2. A six-digit code is generated; its hash, the email, and an expiry are stored
3. The code is emailed; the user submits it
4. Hash, expiry, and attempt counter are verified; a session is issued

Limits: 3 codes per email per hour, 10 per IP per hour, 5 attempts before the code is burned. Proves domain ownership only — faculty and year remain self-declared.

---

## Unified Authentication Endpoint

Web and mobile never talk to USOS, Entra, or the mail provider. They talk to one Uniezz Go API, which dispatches to the right provider internally.

### Public API

| Route | Purpose |
|-------|---------|
| `POST /auth/start` | Body: `{ university, provider? }`. Returns the redirect URL for the chosen provider, or starts the OTP flow |
| `GET /auth/callback/:provider` | Single callback surface. Handles the USOS verifier, the Entra authorization code, and OTP submission |
| `GET /auth/me` | Current session profile |
| `POST /auth/logout` | Clears the Uniezz session |

`:provider` is one of `usos`, `entra`, `otp`.

### Provider Interface

Each provider implements the same contract:

```
AuthProvider {
  start(university)  -> redirect URL or challenge
  callback(params)   -> NormalizedProfile
}
```

### Normalized Profile

Every provider returns the same shape, so the rest of the application never branches on the university:

```
NormalizedProfile {
  provider          usos | entra | otp
  providerUserId    stable ID from the provider
  universityId      umcs | pollub | up | kul | wsei
  email
  firstName, lastName
  photoUrl?
  birthDate?        verified from USOS, otherwise self-declared
  faculty?          verified, inferred, or null
  programme?
  yearOfStudy?      UMCS only
  verification      verified | directory | domain
}
```

### Verification Levels

| Level | Meaning | Source |
|-------|---------|--------|
| `verified` | Academic record confirmed | USOS API — UMCS |
| `directory` | Live account in the university directory | Entra ID — other four |
| `domain` | University email ownership only | Email OTP |

The level is stored on the profile and shown as a badge. Faculty and year are marked as confirmed or self-declared per field, not per account.

### Adding a University

A new university needs a provider entry and a tenant or installation ID. Routes, session handling, and the user model stay untouched. If Politechnika Lubelska, Uniwersytet Przyrodniczy, KUL, or WSEI later grants USOS or Moodle API access, it is swapped in behind the same endpoint and existing accounts are upgraded from `directory` to `verified`.

---

## Session Model

- Provider tokens live in the backend only; the browser never sees them
- The frontend holds a Uniezz session cookie — `HttpOnly`, `Secure`, `SameSite=Lax`
- Profile data is refreshed on login and on demand, not on every request
- Logout clears the Uniezz session; the provider authorization stays until the user revokes it at the source

---

## Security & Privacy

- Consumer secrets, client secrets, and access tokens stored encrypted at rest, kept out of logs
- Requested scopes are the minimum the product needs
- PESEL and other identifiers returned alongside the date of birth are discarded on read and never stored
- Users can disconnect a linked account and delete their Uniezz profile
- Provider authorization is independently revocable by the user in USOSweb or in their Microsoft account
- Personal data handling follows GDPR: stated purpose, minimal scope, deletion on request

---

## Limitations

- Year of study is verified for UMCS only; elsewhere it is always self-declared
- `department` may be empty in any Entra tenant, and this cannot be determined without a live student account
- A tenant administrator can block user consent, forcing that university onto the OTP fallback
- Politechnika Lubelska, Uniwersytet Przyrodniczy, and WSEI keep students and staff in one tenant; separating them relies on the UPN domain or directory attributes, neither of which is confirmed without a live account
- OAuth 1.0a request signing requires a backend; the USOS flow cannot run in the browser alone
- Politechnika Lubelska, Uniwersytet Przyrodniczy, KUL, and WSEI run eHMS, Wirtualny Dziekanat, and e-KUL respectively — none expose a public API

---

## Implementation Checklist

1. Register the USOS application at https://apps.umcs.pl/developers/ and store the credentials as backend secrets
2. Register a multi-tenant Entra application and restrict it to the tenant IDs listed above
3. Build the unified `/auth` routes and the provider interface on the Go API
4. Implement the USOS provider — three-legged OAuth 1.0a with HMAC-SHA1 signing
5. Implement the Entra provider, then test the Graph `$select` against one live account per university and record which fields are populated

---

## References

- USOS API for UMCS — https://apps.umcs.pl/developers/
- Authorization and scopes — https://apps.umcs.pl/developers/api/authorization/
- Installations registry — https://apps.usos.edu.pl/developers/api/definitions/installations/
- Entra app registration — https://learn.microsoft.com/entra/identity-platform/quickstart-register-app
- OIDC protocol — https://learn.microsoft.com/entra/identity-platform/v2-protocols-oidc
- ID token claims — https://learn.microsoft.com/entra/identity-platform/id-token-claims-reference
- Microsoft Graph user — https://learn.microsoft.com/graph/api/user-get
- User consent configuration — https://learn.microsoft.com/entra/identity/enterprise-apps/configure-user-consent

---

*Document: AUTHENTICATION (EN) · Uniezz · v2.3*
