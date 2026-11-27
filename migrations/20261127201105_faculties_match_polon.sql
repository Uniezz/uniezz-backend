-- +goose Up
UPDATE university_faculties SET name = 'Wydział Nauk Społecznych'
WHERE university_id = 'kul' AND name = 'Wydział Nauk Społecznych i Technicznych';

UPDATE university_faculties SET name = 'Wydział Nauk Społecznych i Humanistycznych'
WHERE university_id = 'wspa' AND name = 'Wyższa Szkoła Przedsiębiorczości i Administracji';

INSERT INTO university_faculties (university_id, name, address, latitude, longitude, position)
SELECT n.university_id, n.name, n.address, n.latitude, n.longitude,
    (SELECT COALESCE(MAX(position), 0) FROM university_faculties f WHERE f.university_id = n.university_id) + n.offset_position
FROM (VALUES
    ('kul', 'Wydział Nauk Przyrodniczych i Technicznych', 'ul. Konstantynów 1H', 51.23635, 22.50083, 1),
    ('umlub', 'Wydział Biomedyczny', 'ul. Chodźki 7', 51.259, 22.566, 1),
    ('wspa', 'Wydział Nauk Technicznych', 'ul. Bursaki 12', 51.2702302, 22.5689341, 1)
) AS n(university_id, name, address, latitude, longitude, offset_position)
WHERE EXISTS (SELECT 1 FROM universities u WHERE u.id = n.university_id)
    AND NOT EXISTS (
        SELECT 1 FROM university_faculties f WHERE f.university_id = n.university_id AND f.name = n.name
    );

-- +goose Down
DELETE FROM university_faculties f
WHERE (f.university_id, f.name) IN (
        ('kul', 'Wydział Nauk Przyrodniczych i Technicznych'),
        ('umlub', 'Wydział Biomedyczny'),
        ('wspa', 'Wydział Nauk Technicznych')
    )
    AND NOT EXISTS (SELECT 1 FROM courses c WHERE c.faculty_id = f.id);

UPDATE university_faculties SET name = 'Wyższa Szkoła Przedsiębiorczości i Administracji'
WHERE university_id = 'wspa' AND name = 'Wydział Nauk Społecznych i Humanistycznych';

UPDATE university_faculties SET name = 'Wydział Nauk Społecznych i Technicznych'
WHERE university_id = 'kul' AND name = 'Wydział Nauk Społecznych';
