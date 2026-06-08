-- +goose Up

ALTER TABLE student_absences ADD COLUMN IF NOT EXISTS student_nickname text NULL;

UPDATE student_absences sa
SET student_email    = st.email,
    student_nickname = st.nickname
FROM students st
WHERE st.wcode = sa.wcode
  AND (st.email IS NOT NULL OR st.nickname IS NOT NULL);

-- +goose Down

ALTER TABLE student_absences DROP COLUMN IF EXISTS student_nickname;
