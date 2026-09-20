# Uniezz — Zadania backendu

API w Go. Bez Supabase. Pliki w AWS S3. Web i telefon tylko do tego API.

Szacunki: **0.5d albo 1d**. Sprint = dwa tygodnie, około 10d.

---

## Sprint 1 — Student może się zalogować

**Cel:** Student UMCS i student jednej uczelni Entra mogą wejść. Jest health check i baza.

**Nie w tym sprincie:** feed, czat, Meet, przewodnik, logowanie moderatora.

| ID | Zadanie | d | Zależy | Gotowe, gdy |
|----|---------|---|--------|-------------|
| BE-1.1 | Uruchomić API w Go | 1 | — | `/health` działa. Stary Express usunięty. |
| BE-1.2 | Wybrać bazę i migracje | 1 | BE-1.1 | Można stworzyć i odpalić jedną migrację. Wybór zapisany. |
| BE-1.3 | Tabele user, session, kod z maila | 1 | BE-1.2 | Tabele są. Aplikacja startuje. |
| BE-1.4 | Ciasteczko sesji, „kim jestem”, wylogowanie | 1 | BE-1.3 | Cookie HttpOnly. `/auth/me` i `/auth/logout` działają. |
| BE-1.5 | Jak telefon trzyma login | 0.5 | BE-1.4 | Jest cookie albo token. API to umie. |
| BE-1.6 | Start logowania i powrót z uczelni | 1 | BE-1.4 | Są `/auth/start` i `/auth/callback` dla usos, entra, otp. |
| BE-1.7 | Zarejestrować USOS i schować klucze | 1 | BE-1.6 | Klucze w sekretach, nie w git. |
| BE-1.8 | Dokończyć logowanie USOS | 1 | BE-1.7 | Student UMCS wraca z imieniem i mailem. PESEL nie jest zapisywany. |
| BE-1.9 | Entra i tylko nasze uczelnie | 1 | BE-1.6 | Obcy tenant odrzucony. |
| BE-1.10 | Blokować pracowników Entra | 1 | BE-1.9 | Pracownik nie dostaje sesji studenta. |
| BE-1.11 | Wysłać kod na mail | 1 | BE-1.6 | Tylko mail uczelni. Kod jako hash. |
| BE-1.12 | Sprawdzić kod i limity | 0.5 | BE-1.11 | 3 kody na mail / h, 10 na IP, 5 prób, potem kod ginie. |
| BE-1.13 | Jeden kształt profilu i odznaka | 0.5 | BE-1.8, BE-1.10, BE-1.12 | `/auth/me` pokazuje verified, directory albo domain. |

**Suma: 11.5d** (jeśli nie ma żywego studenta Entra — dowieść jeden tenant w sprincie 2.)

---

## Sprint 2 — Profil, pliki, feed

**Cel:** Student edytuje profil, wrzuca zdjęcie, pisze na feedzie.

| ID | Zadanie | d | Zależy | Gotowe, gdy |
|----|---------|---|--------|-------------|
| BE-2.1 | Czytać i zmieniać tekst profilu | 1 | BE-1.13 | Imię, wydział, rok, zainteresowania, prywatność się zapisują. |
| BE-2.2 | Zdjęcie profilu | 1 | BE-2.1 | URL zdjęcia jest na profilu. |
| BE-2.3 | Bucket S3 i klucze na serwerze | 1 | BE-1.1 | W kliencie nie ma kluczy AWS. |
| BE-2.4 | Krótkie linki do uploadu | 1 | BE-2.3 | Wygasły link nie zapisuje pliku. |
| BE-2.5 | Dodać post na feed | 1 | BE-2.1, BE-2.4 | Ogłoszenie, wydarzenie albo pytanie zapisane. |
| BE-2.6 | Lista postów | 1 | BE-2.5 | Nowe posty wracają. |
| BE-2.7 | Komentarze | 1 | BE-2.6 | Można pisać i czytać. |
| BE-2.8 | Filtr feedu | 1 | BE-2.6 | Filtr po uczelni, typie, dacie. |
| BE-2.9 | Zgłoszenie posta, komentarza albo osoby | 1 | BE-2.7 | Jest wiersz dla moderatora. |
| BE-2.10 | Usunąć konto i odłączyć uczelnię | 1 | BE-2.1 | Profil i sesja znikają. |
| BE-2.11 | Sprawdzić Entra na jednym żywym studencie | 1 | BE-1.10 | Zapisane, które pola Graph wypełnia ta uczelnia. |

**Suma: 11d**

---

## Sprint 3 — Czat

**Cel:** Dwie osoby piszą. Grupy działają. Nowe wiadomości bez odświeżania.

| ID | Zadanie | d | Zależy | Gotowe, gdy |
|----|---------|---|--------|-------------|
| BE-3.1 | Zacząć czat jeden na jeden | 1 | BE-2.1 | Dwie osoby mają jeden wątek. |
| BE-3.2 | Wysłać i pokazać wiadomości | 1 | BE-3.1 | Jest historia. Inni nie czytają. |
| BE-3.3 | Stworzyć grupę | 1 | BE-3.1 | Jest nazwa i członkowie. |
| BE-3.4 | Pisać w grupie | 1 | BE-3.3 | Tylko członkowie czytają i piszą. |
| BE-3.5 | Żywe wiadomości | 1 | BE-3.2 | Drugi klient widzi bez reload. |
| BE-3.6 | Połączenie trzyma | 1 | BE-3.5 | Po zerwaniu znów działa. |
| BE-3.7 | Plik w czacie | 1 | BE-2.4, BE-3.2 | Plik jest na wiadomości. |
| BE-3.8 | Limit rozmiaru i typu | 0.5 | BE-3.7 | Zły plik odrzucony. |
| BE-3.9 | Flaga „nowa wiadomość” | 1 | BE-3.5 | API mówi, że jest nieprzeczytane. |
| BE-3.10 | Otworzyć czat z przyszłego matcha Meet | 1 | BE-3.1 | Endpoint jest. Meet użyje w sprincie 5. |

**Suma: 9.5d**

---

## Sprint 4 — Nauka

**Cel:** Wgrać plik egzaminu, szukać, ocenić kurs.

| ID | Zadanie | d | Zależy | Gotowe, gdy |
|----|---------|---|--------|-------------|
| BE-4.1 | Wgrać plik egzaminu | 1 | BE-2.4 | Plik w S3 i na liście. |
| BE-4.2 | Lista plików | 1 | BE-4.1 | Student widzi pliki kursu. |
| BE-4.3 | Szukanie plików | 1 | BE-4.2 | Szukanie po tytule albo przedmiocie. |
| BE-4.4 | Dane strony kursu | 1 | BE-4.1 | Opis i trudność zapisane. |
| BE-4.5 | Ocenić kurs | 1 | BE-4.4 | Jedna ocena na studenta na kurs. |
| BE-4.6 | Komentarz do kursu | 1 | BE-4.4 | Komentarze na stronie. |
| BE-4.7 | Filtr po uczelni i wydziale | 1 | BE-4.2, BE-4.4 | Filtr działa. |
| BE-4.8 | Filtr po semestrze i przedmiocie | 0.5 | BE-4.7 | Filtr działa. |
| BE-4.9 | Zgłoszenie pliku albo kursu | 0.5 | BE-2.9, BE-4.2 | Ta sama kolejka co feed. |

**Suma: 8d**

---

## Sprint 5 — Meet i przewodnik

**Cel:** Swipe. Match otwiera czat. Zespół pisze miejsca. Studenci czytają i oceniają akademik.

| ID | Zadanie | d | Zależy | Gotowe, gdy |
|----|---------|---|--------|-------------|
| BE-5.1 | Lista kart Meet | 1 | BE-2.2 | Zdjęcie, zainteresowania, uczelnia. |
| BE-5.2 | Like i pass | 1 | BE-5.1 | Wybór zapisany. Pass nie daje matcha. |
| BE-5.3 | Filtr Meet | 1 | BE-5.1 | Uczelnia, rok, cel. |
| BE-5.4 | Match otwiera jeden czat | 1 | BE-3.10, BE-5.2 | Dwa like = jeden wątek. |
| BE-5.5 | Bez zatwierdzonego zdjęcia Meet zamknięty | 1 | BE-5.1 | Brak listy kart. |
| BE-5.6 | Zespół dodaje albo zmienia miejsce | 1 | BE-1.3 | Sklep, spot, akademik, sklepik. |
| BE-5.7 | Zespół chowa albo publikuje miejsce | 1 | BE-5.6 | Ukryte nie jest na liście studentów. |
| BE-5.8 | Studenci czytają przewodnik | 1 | BE-5.7 | Lista wg typu. Nie można dodać miejsca. |
| BE-5.9 | Opinie o akademiku | 1 | BE-5.8 | Można ocenić i skomentować. |
| BE-5.10 | Punkty na mapie | 1 | BE-5.8 | Opublikowane miejsce może mieć pin. |

**Suma: 10d**

---

## Sprint 6 — Moderatorzy

Ekran 08 to tylko szkic. **Nie budować TOTP, dopóki nie ma BE-6.1.**

| ID | Zadanie | d | Zależy | Gotowe, gdy |
|----|---------|---|--------|-------------|
| BE-6.1 | Wpisać logowanie moderatora do auth-doka | 1 | — | Tak/nie dla hasła, TOTP, idle. |
| BE-6.2 | Moderator może wejść | 1 | BE-6.1 | Sesja jak w doku. Student nie może. |
| BE-6.3 | Idle timeout, jeśli dok o to prosi | 1 | BE-6.2 | Bezczynność zabija sesję. Pominąć, jeśli nie. |
| BE-6.4 | Lista zgłoszeń | 1 | BE-2.9 | Moderator widzi kolejkę. |
| BE-6.5 | Zatwierdzić albo odrzucić tekst | 1 | BE-6.4 | Pozycja znika z kolejki. |
| BE-6.6 | Zatwierdzić albo odrzucić media | 1 | BE-6.4, BE-2.4 | Zdjęcie albo plik zostaje albo znika. |
| BE-6.7 | Ostrzec albo zawiesić | 1 | BE-6.5 | Następne logowanie to pokazuje. |
| BE-6.8 | Ban | 0.5 | BE-6.7 | Zbanowany nie wchodzi. |
| BE-6.9 | Dziennik działań | 1 | BE-6.2 | Widać, kto zrobił akcję. |
| BE-6.10 | Moderator przypisany do uczelni | 0.5 | BE-6.2 | Kolejkę można filtrować po uczelni. |

**Suma: 9d**

---

*Dokument: TASKS backend (PL) · Uniezz · v1.1*
