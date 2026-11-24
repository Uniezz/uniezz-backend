-- +goose Up
CREATE TABLE universities (
    id VARCHAR(32) PRIMARY KEY CHECK (id ~ '^[a-z0-9][a-z0-9-]{0,31}$'),
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 3 AND 200),
    auth_type TEXT NOT NULL CHECK (auth_type IN ('USOS', 'OTP')),
    position INT NOT NULL CHECK (position >= 0),
    radon_institution_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER set_universities_updated_at
BEFORE UPDATE ON universities
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE INDEX idx_universities_position ON universities (position, id);

CREATE TABLE university_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    university_id VARCHAR(32) NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    domain TEXT NOT NULL CHECK (domain = LOWER(domain) AND char_length(domain) BETWEEN 3 AND 253),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_university_domains_domain ON university_domains (domain);
CREATE INDEX idx_university_domains_university_id ON university_domains (university_id);

CREATE TABLE university_faculties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    university_id VARCHAR(32) NOT NULL REFERENCES universities(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 3 AND 200),
    address TEXT NOT NULL CHECK (char_length(address) BETWEEN 1 AND 200),
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    position INT NOT NULL CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_university_faculties_university_id_position ON university_faculties (university_id, position);

INSERT INTO universities (id, name, auth_type, position, radon_institution_id) VALUES
    ('umcs', 'Uniwersytet Marii Curie-Skłodowskiej', 'USOS', 1, 'a013df22-d5c5-4871-849c-69e8c6e7e912'),
    ('umlub', 'Uniwersytet Medyczny w Lublinie', 'USOS', 2, '28f45cd0-ed86-4162-baf8-d0add700e59f'),
    ('kul', 'Katolicki Uniwersytet Lubelski Jana Pawła II', 'OTP', 3, '9d3664e2-2c7a-448a-9fb2-278de61e6136'),
    ('up', 'Uniwersytet Przyrodniczy w Lublinie', 'OTP', 4, 'bc05e3e9-35c4-47a4-be34-d13dd35fd0bf'),
    ('pollub', 'Politechnika Lubelska', 'OTP', 5, '337ae4a1-4e95-456f-bc53-d8d54b952a53'),
    ('wspa', 'Wyższa Szkoła Przedsiębiorczości i Administracji w Lublinie', 'OTP', 6, 'a029c48a-4b25-4ee7-bae6-8296a405206b'),
    ('wsei', 'Wyższa Szkoła Ekonomii i Innowacji w Lublinie', 'OTP', 7, 'f4dd5e8d-c071-4ec2-a048-faf9c2aae76d');

INSERT INTO university_domains (university_id, domain) VALUES
    ('kul', 'student.kul.pl'),
    ('up', 'up.lublin.pl'),
    ('pollub', 'pollub.pl'),
    ('wspa', 'wspa.pl'),
    ('wsei', 'wsei.lublin.pl'),
    ('wsei', 'studentwsei.pl');

INSERT INTO university_faculties (university_id, name, address, latitude, longitude, position) VALUES
    ('umcs', 'Wydział Artystyczny', 'al. Kraśnicka 2B', 51.248, 22.5222, 1),
    ('umcs', 'Wydział Biologii i Biotechnologii', 'ul. Akademicka 19', 51.24308, 22.54206, 2),
    ('umcs', 'Wydział Chemii', 'pl. Marii Curie-Skłodowskiej 2', 51.24513, 22.54247, 3),
    ('umcs', 'Wydział Ekonomiczny', 'pl. Marii Curie-Skłodowskiej 5', 51.24564, 22.54028, 4),
    ('umcs', 'Wydział Filologiczny', 'pl. Marii Curie-Skłodowskiej 4A', 51.24466, 22.54097, 5),
    ('umcs', 'Wydział Filozofii i Socjologii', 'pl. Marii Curie-Skłodowskiej 4', 51.24525, 22.54085, 6),
    ('umcs', 'Wydział Historii i Archeologii', 'pl. Marii Curie-Skłodowskiej 4A', 51.24466, 22.54097, 7),
    ('umcs', 'Wydział Matematyki, Fizyki i Informatyki', 'ul. Radziszewskiego 10', 51.2456, 22.5433, 8),
    ('umcs', 'Wydział Nauk o Ziemi i Gospodarki Przestrzennej', 'al. Kraśnicka 2D', 51.24794, 22.52358, 9),
    ('umcs', 'Wydział Pedagogiki i Psychologii', 'ul. Głęboka 43', 51.2467, 22.5251, 10),
    ('umcs', 'Wydział Politologii i Dziennikarstwa', 'ul. Głęboka 45', 51.247, 22.5243, 11),
    ('umcs', 'Wydział Prawa i Administracji', 'pl. Marii Curie-Skłodowskiej 5', 51.24555, 22.5399, 12),
    ('umlub', 'Wydział Lekarski', 'ul. Chodźki 19', 51.26318, 22.56379, 1),
    ('umlub', 'Wydział Lekarsko-Dentystyczny', 'ul. Chodźki 19', 51.26318, 22.56379, 2),
    ('umlub', 'Wydział Farmaceutyczny', 'ul. Chodźki 1', 51.25963, 22.56781, 3),
    ('umlub', 'Wydział Nauk Medycznych', 'ul. Chodźki 7', 51.259, 22.566, 4),
    ('umlub', 'Wydział Nauk o Zdrowiu', 'ul. Staszica 4/6', 51.24859, 22.56194, 5),
    ('kul', 'Wydział Teologii', 'Al. Racławickie 14', 51.24795, 22.54485, 1),
    ('kul', 'Wydział Prawa, Prawa Kanonicznego i Administracji', 'ul. Spokojna 1', 51.2494, 22.5542, 2),
    ('kul', 'Wydział Filozofii', 'Al. Racławickie 14', 51.24795, 22.54485, 3),
    ('kul', 'Wydział Nauk Humanistycznych', 'Al. Racławickie 14', 51.24795, 22.54485, 4),
    ('kul', 'Wydział Nauk Społecznych i Technicznych', 'Al. Racławickie 14', 51.24795, 22.54485, 5),
    ('kul', 'Wydział Medyczny', 'ul. Konstantynów 1I', 51.23635, 22.50083, 6),
    ('up', 'Wydział Agrobioinżynierii', 'ul. Akademicka 13', 51.2438, 22.54049, 1),
    ('up', 'Wydział Medycyny Weterynaryjnej', 'ul. Głęboka 30', 51.2409, 22.5438, 2),
    ('up', 'Wydział Nauk o Zwierzętach i Biogospodarki', 'ul. Akademicka 13', 51.2438, 22.54049, 3),
    ('up', 'Wydział Ogrodnictwa i Architektury Krajobrazu', 'ul. Akademicka 13', 51.2438, 22.54049, 4),
    ('up', 'Wydział Inżynierii Produkcji', 'ul. Głęboka 28', 51.2409, 22.5451, 5),
    ('up', 'Wydział Nauk o Żywności i Biotechnologii', 'ul. Skromna 8', 51.24938, 22.51063, 6),
    ('up', 'Wydział Biologii Środowiskowej', 'ul. Akademicka 13', 51.2438, 22.54049, 7),
    ('pollub', 'Wydział Mechaniczny', 'ul. Nadbystrzycka 36', 51.2371, 22.5496, 1),
    ('pollub', 'Wydział Elektrotechniki i Informatyki', 'ul. Nadbystrzycka 38A', 51.23677, 22.54877, 2),
    ('pollub', 'Wydział Budownictwa i Architektury', 'ul. Nadbystrzycka 40', 51.2346, 22.5505, 3),
    ('pollub', 'Wydział Inżynierii Środowiska i Energetyki', 'ul. Nadbystrzycka 40B', 51.2342, 22.5504, 4),
    ('pollub', 'Wydział Zarządzania', 'ul. Nadbystrzycka 38', 51.23525, 22.54873, 5),
    ('pollub', 'Wydział Matematyki i Informatyki Technicznej', 'ul. Nadbystrzycka 38', 51.23522, 22.54925, 6),
    ('wspa', 'Wyższa Szkoła Przedsiębiorczości i Administracji', 'ul. Bursaki 12', 51.2702302, 22.5689341, 1),
    ('wsei', 'Wydział Administracji i Nauk Społecznych', 'ul. Projektowa 4', 51.24552, 22.61056, 1),
    ('wsei', 'Wydział Nauk o Zdrowiu', 'ul. Projektowa 4', 51.24552, 22.61056, 2),
    ('wsei', 'Wydział Psychologii i Pedagogiki', 'ul. Projektowa 4', 51.24552, 22.61056, 3),
    ('wsei', 'Wydział Transportu i Informatyki', 'ul. Projektowa 4', 51.24552, 22.61056, 4);

ALTER TABLE users
    ADD CONSTRAINT fk_users_university_id FOREIGN KEY (university_id) REFERENCES universities(id);
ALTER TABLE otp_requests
    ADD CONSTRAINT fk_otp_requests_university_id FOREIGN KEY (university_id) REFERENCES universities(id);
ALTER TABLE oauth_states
    ADD CONSTRAINT fk_oauth_states_university_id FOREIGN KEY (university_id) REFERENCES universities(id);

-- +goose Down
ALTER TABLE oauth_states DROP CONSTRAINT fk_oauth_states_university_id;
ALTER TABLE otp_requests DROP CONSTRAINT fk_otp_requests_university_id;
ALTER TABLE users DROP CONSTRAINT fk_users_university_id;
DROP TABLE university_faculties;
DROP TABLE university_domains;
DROP TABLE universities;
