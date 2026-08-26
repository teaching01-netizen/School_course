-- +goose Up
ALTER TABLE crm_cross_study_assignments
  ADD COLUMN expanded_destination_course_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
  ADD COLUMN expanded_enrollment_created_ids uuid[] NOT NULL DEFAULT '{}'::uuid[];

UPDATE crm_cross_study_assignments
SET expanded_destination_course_ids = CASE
      WHEN dest_course_a_id = dest_course_b_id THEN ARRAY[dest_course_a_id]
      ELSE ARRAY[dest_course_a_id, dest_course_b_id]
    END,
    expanded_enrollment_created_ids = array_remove(ARRAY[
      CASE WHEN dest_course_a_enrollment_created THEN dest_course_a_id END,
      CASE WHEN dest_course_b_enrollment_created AND dest_course_b_id <> dest_course_a_id THEN dest_course_b_id END
    ], NULL)::uuid[];

-- +goose Down
ALTER TABLE crm_cross_study_assignments
  DROP COLUMN expanded_enrollment_created_ids,
  DROP COLUMN expanded_destination_course_ids;
