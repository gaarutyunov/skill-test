-- Integration-test fixtures: seed classes, sections and a couple of students
-- (role_id = 3) so the report service has real data to fetch from the Node API.
-- Runs after 01-tables.sql and 02-seed-db.sql.

INSERT INTO classes (name, sections) VALUES ('Grade 10', 'A,B')
    ON CONFLICT (name) DO NOTHING;
INSERT INTO sections (name) VALUES ('A'), ('B')
    ON CONFLICT (name) DO NOTHING;

-- Student 1 — full profile (exercises every report field).
INSERT INTO users (name, email, role_id, created_dt, is_active, is_email_verified)
VALUES ('Alice Johnson', 'alice.johnson@school-example.com', 3, now(), true, true);
INSERT INTO user_profiles
    (user_id, gender, dob, phone, class_name, section_name, roll, admission_dt,
     father_name, father_phone, mother_name, mother_phone,
     guardian_name, guardian_phone, relation_of_guardian,
     current_address, permanent_address)
VALUES
    ((SELECT id FROM users WHERE email = 'alice.johnson@school-example.com'),
     'Female', '2008-05-14', '5551234567', 'Grade 10', 'A', 12, '2020-01-10',
     'Robert Johnson', '5559876543', 'Mary Johnson', '5558765432',
     'Robert Johnson', '5559876543', 'Father',
     '123 Maple Street', '123 Maple Street');

-- Student 2 — minimal profile (exercises nullable fields).
INSERT INTO users (name, email, role_id, created_dt, is_active, is_email_verified)
VALUES ('Bob Smith', 'bob.smith@school-example.com', 3, now(), true, true);
INSERT INTO user_profiles (user_id, gender, class_name, section_name, roll)
VALUES
    ((SELECT id FROM users WHERE email = 'bob.smith@school-example.com'),
     'Male', 'Grade 10', 'B', 7);
