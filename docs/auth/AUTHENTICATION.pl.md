# Uniezz — Autoryzacja

## Przegląd

Uniezz weryfikuje, czy użytkownik jest rzeczywistym, aktywnym studentem, zanim przyzna mu dostęp. Pięć uczelni, trzech dostawców tożsamości, jeden punkt wejścia na **Go API** dla webu i mobile.

| Uczelnia | Dostawca | Zweryfikowane dane |
|----------|----------|--------------------|
| **UMCS** | USOS API (OAuth 1.0a) | Tożsamość, wydział, kierunek, rok |
| **Politechnika Lubelska** | Microsoft Entra ID (OIDC) | Tożsamość, uczelnia; wydział, jeśli opublikowany |
| **Uniwersytet Przyrodniczy** | Microsoft Entra ID (OIDC) | Tożsamość, uczelnia; wydział, jeśli opublikowany |
| **KUL** | Microsoft Entra ID (OIDC) | Tożsamość, uczelnia; wydział, jeśli opublikowany |
| **WSEI** | Microsoft Entra ID (OIDC) | Tożsamość, uczelnia; wydział, jeśli opublikowany |
| *dowolna z pięciu* | Email OTP | Wyłącznie domena uczelniana — rozwiązanie zapasowe |

### Rozważane podejścia

| Podejście | Decyzja |
|-----------|---------|
| **USOS API (OAuth 1.0a)** | Wybrane dla UMCS. Oficjalne, zweryfikowane dane, samodzielna rejestracja |
| **Microsoft Entra ID (OIDC)** | Wybrane dla pozostałych czterech. Wszystkie pięć uczelni ma tenanty Entra |
| **Email OTP na domenie uczelnianej** | Rozwiązanie zapasowe, gdy administrator blokuje zgodę na aplikację |
| Moodle Web Services | Odrzucone. Token wydawany ręcznie przez dział IT uczelni |
| Scraping formularza logowania kampusu | Odrzucone. Obsługa haseł uczelnianych to ryzyko prawne i bezpieczeństwa |

---

## Dostawca 1 — UMCS przez USOS API

UMCS działa na **USOS** — systemie obsługi studiów używanym przez ponad 50 polskich uczelni — i udostępnia publiczne **USOS API**. To jedyna lubelska uczelnia w oficjalnym rejestrze instalacji USOS API.

### Rejestracja aplikacji

Samoobsługowo — bez procedury zatwierdzania i bez umowy z uczelnią.

1. Otworzyć https://apps.umcs.pl/developers/
2. Podać nazwę aplikacji, opcjonalnie adres strony oraz działający adres e-mail do kontaktu
3. Otrzymać **Consumer Key** i **Consumer Secret**

Consumer Secret nie opuszcza backendu i nie trafia do repozytorium. Pod tym samym adresem dostępny jest formularz unieważnienia klucza, a potwierdzenie trafia na zarejestrowany adres e-mail.

### Przepływ OAuth 1.0a

USOS API używa OAuth 1.0a (trójetapowego), nie OAuth 2.0. Podpisywanie żądań odbywa się w całości po stronie backendu algorytmem HMAC-SHA1.

| Krok | URL |
|------|-----|
| Request token | `https://apps.umcs.pl/services/oauth/request_token` |
| Authorize | `https://apps.umcs.pl/services/oauth/authorize` |
| Access token | `https://apps.umcs.pl/services/oauth/access_token` |

1. **Request token** — backend wywołuje `request_token` z Consumer Key, listą `scopes` oraz `oauth_callback` wskazującym na Uniezz
2. **Przekierowanie** — użytkownik trafia na `authorize` i loguje się danymi UMCS na stronie uczelni
3. **Zgoda** — użytkownik zatwierdza uprawnienia; USOS przekierowuje go z `oauth_token` i `oauth_verifier`
4. **Access token** — backend wymienia verifier na długoterminowy token dostępu wraz z sekretem
5. **Pobranie profilu** — backend wywołuje API podpisanym tokenem i tworzy lub aktualizuje konto Uniezz

Hasło do UMCS wpisywane jest wyłącznie na stronie UMCS i nigdy nie jest widoczne dla Uniezz.

### Zakresy uprawnień (scopes)

Żaden z nich nie wymaga zgody administratora.

| Scope | Zastosowanie w Uniezz |
|-------|-----------------------|
| *(domyślny)* | Podstawowa tożsamość — identyfikator, imię i nazwisko |
| `studies` | Programy, przedmioty, listy grup — źródło wydziału, kierunku i roku studiów |
| `email` | Uczelniany adres e-mail, używany jako identyfikator konta |
| `photo` | Zdjęcie profilowe i ustawienia jego widoczności |
| `personal` | Data urodzenia — ograniczenia wiekowe, urodziny, filtry wieku w module Znajomości |
| `offline_access` | Długoterminowy token pozwalający odświeżać profil między sesjami |

**O zakresie `personal`:** żądany jest ze względu na datę urodzenia, potrzebną Uniezz do ograniczeń wiekowych oraz filtrów wieku w module Znajomości. Ten sam zakres zwraca PESEL i inne identyfikatory — są one odrzucane przy odczycie i nigdy nie trafiają do bazy ani do logów.

**Świadomie nieżądane:** `grades`, `payments`, `mobile_numbers`. Produkt ich nie potrzebuje, a ich żądanie obniża odsetek udzielonych zgód.

### Pobierane dane

Odczytywane przez `services/users/user`, gdzie parametr `fields` jawnie wymienia każde pole (USOS API tego wymaga): identyfikator użytkownika, imię i nazwisko, uczelniany e-mail, data urodzenia, programy studiów, status studenta, URL zdjęcia. Dane wydziału ustalane są na podstawie programu przez odpowiednie endpointy. Dokładny zestaw pól zostanie ustalony na etapie integracji w oparciu o https://apps.umcs.pl/developers/api/

---

## Dostawca 2 — Entra ID dla pozostałych czterech

Wszystkie pięć uczelni prowadzi tenanty Microsoft Entra ID — potwierdzone przez endpoint wykrywania tenantów Microsoft. Dla czterech uczelni bez publicznego API akademickiego Entra jest najsilniejszym dostępnym potwierdzeniem: poświadcza aktywne konto w katalogu uczelni, a nie samo posiadanie adresu e-mail.

### Tenanty

| Uczelnia | Domeny | Tenant ID | Typ |
|----------|--------|-----------|-----|
| Politechnika Lubelska | `pollub.edu.pl`, `student.pollub.edu.pl` | `dbb41d7a-0043-4ee2-9843-6e4ff66cc9c8` | Managed |
| Uniwersytet Przyrodniczy | `up.lublin.pl`, `student.`, `stud.` | `25a59194-f151-40c5-9e45-365a4d46d7b9` | Managed |
| WSEI | `wsei.lublin.pl`, `student.`, `stud.` | `bab1e1e4-b3e8-49c1-93e4-eaeff72f1f5d` | Managed |
| KUL — studenci | `student.kul.pl` | `f445c1ee-43fc-42e8-b642-b382d382c3c1` | Managed — **dopuszczony** |
| KUL — pracownicy | `kul.pl` | `7952c9f0-b177-4a4c-ab55-7c2f5ab0808d` | Managed — **odrzucany** |
| UMCS | `umcs.pl` | `80dbd34a-9b20-490b-ac49-035af103ab2b` | Federated (własny IdP SAML) |

KUL prowadzi dwa osobne tenanty i jest jedyną uczelnią, gdzie studenci są już oddzieleni na poziomie tenanta: tenant studencki jest dopuszczony, tenant pracowniczy odrzucany. UMCS wymieniono dla kompletności — loguje się przez USOS.

### Konfiguracja

Rejestrowana jest jedna aplikacja **multi-tenant** w Entra ID, a następnie dopuszczane są wyłącznie tenant ID oznaczone wyżej jako dopuszczone. Użytkownik, którego `tid` nie znajduje się na liście, zostaje odrzucony — to ogranicza logowanie do lubelskich uczelni.

### Wyłącznie studenci

Uniezz to platforma studencka — konta pracownicze nie mogą się logować. Samo tenant ID tego nie zapewnia, ponieważ Politechnika Lubelska, Uniwersytet Przyrodniczy i WSEI trzymają studentów oraz pracowników w jednym tenancie. Ograniczenie jest warstwowe:

1. **Biała lista tenantów** — całkowicie rozstrzyga sprawę KUL, którego studenci mają własny tenant
2. **Domena UPN** — tam, gdzie uczelnia wydaje pocztę studencką na subdomenie (`student.`, `stud.`), dopuszczane są wyłącznie takie adresy
3. **Atrybuty katalogu** — sprawdzane są `employeeType` i `jobTitle` z Graph; wartość wskazująca na pracownika jest odrzucana
4. **Weryfikacja ręczna** — jeśli żaden z powyższych kroków nie rozstrzyga, konto powstaje z ograniczonymi uprawnieniami i trafia do przeglądu

Kroki 2 i 3 zależą od tego, jak każda uczelnia zakłada konta, czego nie da się ustalić bez aktywnego konta. Dopóki nie zostanie to potwierdzone dla każdej uczelni, podział na studentów i pracowników działa pewnie wyłącznie dla KUL oraz — przez `student_status` — dla UMCS.

### Gwarantowane claims

Dostępne z tokenu ID bez dodatkowego zapytania:

| Claim | Zastosowanie |
|-------|--------------|
| `oid` | Stabilny identyfikator użytkownika w tenancie — klucz główny |
| `tid` | Identyfikator tenanta — jednoznacznie wskazuje uczelnię |
| `name` | Imię i nazwisko |
| `preferred_username` | UPN, zwykle uczelniany e-mail |
| `email` | E-mail, jeśli opublikowany przez administratora; w przeciwnym razie używany jest UPN |

### Opcjonalne pola profilu

Pobierane z Microsoft Graph z zakresem `User.Read`, na który zgodę wyraża sam użytkownik — bez udziału administratora. Domyślna odpowiedź `/me` **nie zawiera** pola `department`, dlatego wymagany jest `$select`:

```
GET https://graph.microsoft.com/v1.0/me?$select=id,displayName,mail,userPrincipalName,jobTitle,department,companyName,officeLocation,employeeType
```

- `department` — zwykle wydział
- `jobTitle`, `employeeType` — czasem „Student"
- `companyName` — nazwa uczelni
- `officeLocation` — budynek lub kampus
- `GET /me/photo/$value` — zdjęcie profilowe, ten sam zakres
- `birthday` — data urodzenia; obecna w schemacie, ale u studentów praktycznie nigdy niewypełniona, więc traktowana jako zadeklarowana przy onboardingu

Pola te wypełnia administrator uczelni przy zakładaniu konta. Mogą być kompletne albo puste, a każdy tenant zachowuje się inaczej. Sprawdzenie tego wymaga jednego aktywnego konta studenckiego na uczelnię.

**Nigdy niedostępne:** rok studiów, numer grupy, semestr, oceny. Te dane znajdują się w systemach dziekanatów (eHMS, Wirtualny Dziekanat, e-KUL), z których żaden nie udostępnia publicznego API.

---

## Dostawca 3 — Zapasowy Email OTP

Stosowany, gdy administrator uczelni wyłączył zgodę użytkowników na zewnętrzne aplikacje multi-tenant — wtedy zamiast ekranu zgody Entra zwraca „Need admin approval".

1. Użytkownik podaje uczelniany e-mail; domena sprawdzana jest z białą listą
2. Generowany jest sześciocyfrowy kod; zapisywane są jego skrót, e-mail i termin ważności
3. Kod wysyłany jest mailem; użytkownik go wprowadza
4. Weryfikowane są skrót, termin i licznik prób; wydawana jest sesja

Limity: 3 kody na adres na godzinę, 10 na IP na godzinę, 5 prób przed unieważnieniem kodu. Potwierdza wyłącznie posiadanie domeny — wydział i rok pozostają zadeklarowane przez użytkownika.

---

## Jednolity endpoint autoryzacji

Web i mobile nigdy nie komunikują się z USOS, Entra ani dostawcą poczty. Rozmawiają z jednym Go API Uniezz, które wewnętrznie kieruje żądanie do właściwego dostawcy.

### Publiczne API

| Trasa | Przeznaczenie |
|-------|---------------|
| `POST /auth/start` | Treść: `{ university, provider? }`. Zwraca URL przekierowania wybranego dostawcy lub rozpoczyna przepływ OTP |
| `GET /auth/callback/:provider` | Jeden punkt zwrotny. Obsługuje verifier z USOS, kod autoryzacyjny Entra oraz wprowadzenie OTP |
| `GET /auth/me` | Profil bieżącej sesji |
| `POST /auth/logout` | Czyści sesję Uniezz |

`:provider` to jedno z `usos`, `entra`, `otp`.

### Interfejs dostawcy

Każdy dostawca realizuje ten sam kontrakt:

```
AuthProvider {
  start(university)  -> URL przekierowania lub challenge
  callback(params)   -> NormalizedProfile
}
```

### Znormalizowany profil

Wszyscy dostawcy zwracają tę samą strukturę, dzięki czemu reszta aplikacji nie rozgałęzia się według uczelni:

```
NormalizedProfile {
  provider          usos | entra | otp
  providerUserId    stabilny identyfikator od dostawcy
  universityId      umcs | pollub | up | kul | wsei
  email
  firstName, lastName
  photoUrl?
  birthDate?        zweryfikowana z USOS, w przeciwnym razie zadeklarowana
  faculty?          zweryfikowany, wywnioskowany lub null
  programme?
  yearOfStudy?      tylko UMCS
  verification      verified | directory | domain
}
```

### Poziomy weryfikacji

| Poziom | Znaczenie | Źródło |
|--------|-----------|--------|
| `verified` | Potwierdzony wpis w systemie studiów | USOS API — UMCS |
| `directory` | Aktywne konto w katalogu uczelni | Entra ID — pozostałe cztery |
| `domain` | Wyłącznie posiadanie uczelnianego e-maila | Email OTP |

Poziom zapisywany jest w profilu i prezentowany jako odznaka. Wydział i rok oznaczane są jako potwierdzone lub zadeklarowane osobno dla każdego pola, a nie dla całego konta.

### Dodanie uczelni

Nowa uczelnia wymaga wpisu dostawcy oraz identyfikatora tenanta lub instalacji. Trasy, obsługa sesji i model użytkownika pozostają bez zmian. Jeśli Politechnika Lubelska, Uniwersytet Przyrodniczy, KUL lub WSEI udostępnią później API USOS albo Moodle, zostanie ono podłączone za tym samym endpointem, a istniejące konta awansują z `directory` do `verified`.

---

## Model sesji

- Tokeny dostawców przechowywane są wyłącznie w backendzie, przeglądarka ich nie widzi
- Frontend korzysta z ciasteczka sesji Uniezz — `HttpOnly`, `Secure`, `SameSite=Lax`
- Dane profilu odświeżane są przy logowaniu i na żądanie, nie przy każdym zapytaniu
- Wylogowanie czyści sesję Uniezz; autoryzacja u dostawcy pozostaje do czasu jej cofnięcia u źródła

---

## Bezpieczeństwo i prywatność

- Consumer secret, client secret i tokeny dostępu przechowywane w postaci zaszyfrowanej, nietrafiające do logów
- Żądany jest minimalny zakres uprawnień potrzebny produktowi
- PESEL i inne identyfikatory zwracane wraz z datą urodzenia są odrzucane przy odczycie i nigdy nieprzechowywane
- Użytkownik może odłączyć powiązane konto i usunąć profil Uniezz
- Autoryzację u dostawcy użytkownik może niezależnie cofnąć w USOSweb lub na swoim koncie Microsoft
- Przetwarzanie danych osobowych zgodne z RODO: określony cel, minimalny zakres, usunięcie na żądanie

---

## Ograniczenia

- Rok studiów jest zweryfikowany wyłącznie dla UMCS; w pozostałych przypadkach zawsze deklarowany przez użytkownika
- Pole `department` może być puste w dowolnym tenancie Entra, a bez aktywnego konta studenckiego nie da się tego ustalić
- Administrator tenanta może zablokować zgodę użytkowników, co przenosi daną uczelnię na zapasowy OTP
- Politechnika Lubelska, Uniwersytet Przyrodniczy i WSEI trzymają studentów oraz pracowników w jednym tenancie; ich rozdzielenie opiera się na domenie UPN lub atrybutach katalogu, z których żadne nie jest potwierdzone bez aktywnego konta
- Podpisywanie żądań OAuth 1.0a wymaga backendu; przepływu USOS nie da się zrealizować wyłącznie w przeglądarce
- Politechnika Lubelska, Uniwersytet Przyrodniczy, KUL i WSEI działają odpowiednio na eHMS, Wirtualnym Dziekanacie i e-KUL — żaden z tych systemów nie udostępnia publicznego API

---

## Lista zadań wdrożeniowych

1. Zarejestrować aplikację USOS na https://apps.umcs.pl/developers/ i umieścić klucze w sekretach backendu
2. Zarejestrować aplikację multi-tenant w Entra i ograniczyć ją do wymienionych wyżej tenant ID
3. Zbudować jednolite trasy `/auth` oraz interfejs dostawcy na Go API
4. Zaimplementować dostawcę USOS — trójetapowy OAuth 1.0a z podpisem HMAC-SHA1
5. Zaimplementować dostawcę Entra, następnie przetestować `$select` z Graph na jednym aktywnym koncie każdej uczelni i zapisać, które pola są wypełnione

---

## Źródła

- USOS API dla UMCS — https://apps.umcs.pl/developers/
- Autoryzacja i zakresy uprawnień — https://apps.umcs.pl/developers/api/authorization/
- Rejestr instalacji — https://apps.usos.edu.pl/developers/api/definitions/installations/
- Rejestracja aplikacji Entra — https://learn.microsoft.com/entra/identity-platform/quickstart-register-app
- Protokół OIDC — https://learn.microsoft.com/entra/identity-platform/v2-protocols-oidc
- Claims w tokenie ID — https://learn.microsoft.com/entra/identity-platform/id-token-claims-reference
- Użytkownik w Microsoft Graph — https://learn.microsoft.com/graph/api/user-get
- Konfiguracja zgody użytkowników — https://learn.microsoft.com/entra/identity/enterprise-apps/configure-user-consent

---

*Dokument: AUTHENTICATION (PL) · Uniezz · v2.3*
