# Auth Architecture Brief: USOS & Sessions

## 1. USOS Integration Flow 
**Handshake:** Backend requests a temporary token from USOS -> Redirects user to UMCS login -> User consents -> USOS redirects back with oauth_verifier -> Backend exchanges it for a permanent Access Token.

**Data Retrieval:** We query `services/users/user` (using `studies` and `personal` scopes) to get the student's ID, name, email, and faculty. We discard any sensitive data like PESEL immediately.

## 2. Session & Token Lifecycle
   **Creation:** After the USOS callback, `upsert` the user in Postgres. Generate a 32-byte Refresh Token, hash it, and save it in the sessions table.

   **Delivery:** Send the Refresh Token via an `HttpOnly` and `Secure` cookie. Return a short-lived JWT (15 mins) in the JSON response.

   **Edge Cases:**

*JWT Expiry:* Frontend catches 401 and calls `/auth/refresh`.

*Logout*: Delete the specific session from Postgres and send an expired clearing cookie

*Cleanup*: Run a daily database job to delete expired sessions

## 3. Proposed API Contract

| Method | Endpoint | Purpose |
| - | - | - |
| `POST` | `/auth/start` | Initiates flow, returns USOS/Entra redirect URL |
| `GET` | `/auth/callback/` | Handles callback, sets Refresh Cookie, returns JWT |
| `GET` | `/auth/me` | Validates JWT, returns `NormalizedProfile` |
| `POST` | `/auth/refresh` | Validates Cookie, issues a new JWT |
| `POST` | `/auth/logout` | Deletes DB session, clears Cookie |

## 4. University Scoping 
**Database:** Use a composite unique constraint on the users table: `UNIQUE(university_id, provider_user_id)`.

**Application:** Embed `university_id` in the JWT claims. A Go middleware must extract it and inject it into the request context. The repository layer must append `WHERE university_id = ?` to all database queries to prevent cross-tenant data leaks.

## 5. Recommended Go Libraries
[`github.com/dghubble/oauth1`](https://github.com/dghubble/oauth1) (OAuth 1.0a client).

[`github.com/golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt/v5) (Stateless JWT generation/validation).

[`github.com/google/uuid`](https://github.com/google/uuid) (For generating primary keys and secure tokens).