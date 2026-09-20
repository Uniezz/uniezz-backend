# Uniezz — Opis produktu

## Przegląd

**Uniezz** to aplikacja webowa dla studentów lubelskich uczelni wyższych. Platforma łączy studentów pięciu wydziałów akademickich miasta w jednej przestrzeni: komunikacja, wymiana wiedzy, praktyczne informacje o życiu w Lublinie oraz poznawanie nowych osób.

### Docelowe uczelnie

- **UMCS** — Uniwersytet Marii Curie-Skłodowskiej
- **KUL** — Katolicki Uniwersytet Lubelski Jana Pawła II
- **Politechnika Lubelska** — Politechnika Lubelska
- **Uniwersytet Przyrodniczy** — Uniwersytet Przyrodniczy w Lublinie
- **WSEI** — Wyższa Szkoła Ekonomii i Innowacji w Lublinie

---

## Główne moduły

### 1. Autoryzacja

Logowanie za pomocą kont uczelnianych (prawdopodobnie **Moodle** i analogiczne systemy każdej uczelni). Gwarantuje to, że dostęp do platformy mają wyłącznie aktywni studenci.

- Jednolite logowanie (SSO) przez integrację z systemami uczelni
- Profil powiązany z konkretną uczelnią, wydziałem i rokiem studiów
- Weryfikacja statusu studenta

### 2. Profil i kontakty

- Profil osobisty: zdjęcie, imię, uczelnia, wydział, rok studiów, zainteresowania
- Lista kontaktów (znajomi, koledzy z roku, znajome osoby)
- Ustawienia prywatności
- Możliwość znajdowania studentów własnej i innych lubelskich uczelni

### 3. Tablica postów (ogłoszenia)

Społecznościowa tablica dla studentów:

- **Ogłoszenia** — kupno/sprzedaż, poszukiwanie współlokatora, usługi
- **Wydarzenia** — imprezy, spotkania, konferencje, wydarzenia sportowe
- **Pytania** — pomoc w nauce, sprawy codzienne, porady dla nowych studentów
- **Komentarze** — dyskusja pod każdym postem
- Filtrowanie według uczelni, kategorii i daty

### 4. Czat (komunikator)

Wbudowany komunikator do rozmów prywatnych i grupowych:

- Wiadomości prywatne między użytkownikami
- Czaty grupowe (według przedmiotu, wydziału, akademika itp.)
- Powiadomienia o nowych wiadomościach
- Wysyłanie tekstu i plików multimedialnych (z moderacją)

### 5. Nauka i pomoc studentom

Sekcja wsparcia akademickiego:

- **Egzaminy z poprzednich lat** — archiwum zadań, pytań egzaminacyjnych i testów
- **Oceny i opinie** — rankingi oraz komentarze o wykładowcach i przedmiotach
- **Opisy przedmiotów** — program, poziom trudności, rekomendacje do nauki
- Wyszukiwanie i filtrowanie według uczelni, wydziału, semestru i przedmiotu
- Możliwość dodawania komentarzy i uzupełniania materiałów

### 6. Przewodnik studenta (podstawowe informacje)

Praktyczny przewodnik po życiu studenckim w Lublinie:

- **Zakupy** — gdzie kupować taniej, porównanie sklepów (Biedronka, Lidl, Carrefour itp.)
- **Miejsca na czas wolny** — kawiarnie, bary, parki, miejsca kulturalne
- **Akademiki** — opis, koszty zamieszkania, warunki, opinie studentów
- **Sklepy w pobliżu** — co gdzie kupić (żywność, artykuły papiernicze, elektronika, ubrania)
- Mapa z oznaczonymi punktami zainteresowania

### 7. Poznawanie (Tinder-like)

Moduł do szukania znajomych, towarzystwa i relacji:

- Karty użytkowników ze zdjęciem, zainteresowaniami i uczelnią
- Mechanika swipe: lajk / pomiń
- Match przy wzajemnym zainteresowaniu — otwiera się czat
- Filtry: uczelnia, rok studiów, cel (znajomi / relacje / towarzystwo)
- Ścisła moderacja profili i zdjęć

---

## Wielojęzyczność

Aplikacja obsługuje cztery języki:

| Język | Kod |
|-------|-----|
| Polski | `pl` |
| Ukraiński | `uk` |
| Rosyjski | `ru` |
| Angielski | `en` |

- Przełączanie języka w ustawieniach profilu
- Lokalizacja całego interfejsu
- Treści użytkowników wyświetlane w języku autora; interfejs w wybranym języku

---

## Moderacja i bezpieczeństwo

Ścisła moderacja treści to kluczowa zasada platformy:

- **Teksty** — automatyczna i ręczna kontrola pod kątem obelg, rasizmu, dyskryminacji, hejtu i negatywnych treści
- **Media** — kontrola przesyłanych zdjęć i filmów
- **Zgłoszenia** — użytkownicy mogą zgłaszać naruszenia
- **Sankcje** — ostrzeżenia, czasowa blokada, stały ban
- **Moderatorzy** — zespół przedstawicieli każdej uczelni

---

## Wartość dla studentów

| Problem | Rozwiązanie w Uniezz |
|---------|----------------------|
| Rozproszone czaty i grupy w mediach społecznościowych | Jedna platforma dla wszystkich lubelskich uczelni |
| Trudno znaleźć materiały do egzaminów | Archiwum egzaminów i opinie o przedmiotach |
| Nowy student nie wie, gdzie mieszkać i co kupować | Przewodnik z aktualnymi informacjami |
| Trudno poznać ludzi poza własną uczelnią | Moduł poznawania między studentami miasta |
| Toksyczne treści w otwartych grupach | Ścisła moderacja i weryfikacja studentów |

---

## Stack technologiczny (planowany)

- **Frontend:** React + TypeScript + Vite + TanStack Router (web); Expo / React Native (mobile)
- **Backend:** Go — własne API. Bez Supabase i innych BaaS; web i mobile łączą się wyłącznie z tym API
- **Autoryzacja:** USOS (OAuth 1.0a), Microsoft Entra ID (OIDC), email OTP jako zapas — zob. `docs/auth/`
- **Media:** AWS S3 — zdjęcia, załączniki w czacie, pliki egzaminów i inne przesyłane treści
- **Baza danych:** TBD
- **Hosting:** TBD

---

*Dokument: PRODUCT_DESCRIPTION (PL) · Uniezz · v1.2*
