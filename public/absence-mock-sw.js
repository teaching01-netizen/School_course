/* Temporary dev-only service worker for the live preview walkthrough.
   Intercepts only /api/* and answers with canned absence-form data so every
   screen state can be exercised without the Go backend. Remove this file and
   unregister the worker when the walkthrough is done. */

const INST = 7 * 60 * 60_000; // Asia/Bangkok offset, the institute zone
const pad = (n) => String(n).padStart(2, "0");
const isoNow = (ms) => new Date(ms).toISOString();
const dateKey = (offsetDays) =>
  new Date(Date.now() + INST + offsetDays * 86_400_000).toISOString().slice(0, 10);
const timeISO = (d, h, m = 0) =>
  new Date(`${d}T${pad(h)}:${pad(m)}:00+07:00`).toISOString();

const missedDate = dateKey(1); // tomorrow, institute time

const missedSession = {
  id: "missed-math-1",
  start_at: timeISO(missedDate, 9, 0),
  end_at: timeISO(missedDate, 10, 0),
  date: missedDate,
  already_absent: false,
};

const sitInSessions = [
  {
    id: "sitin-math-1",
    course_id: "course-sitin",
    start_at: timeISO(missedDate, 10, 15),
    end_at: timeISO(missedDate, 11, 15),
    class_name: "Make-up A",
  },
  {
    id: "sitin-math-2",
    course_id: "course-sitin",
    start_at: timeISO(missedDate, 13, 30),
    end_at: timeISO(missedDate, 14, 30),
    class_name: "Make-up B",
  },
];

const sessionsInRange = {
  subjects: [
    {
      subject_id: "subject-math",
      subject_code: "MATH",
      subject_name: "Mathematics",
      course_id: "course-math",
      course_code: "MATH101",
      course_name: "Mathematics 101",
      absence_rate_exceeded: false,
      remaining_absence_days: 10,
      sessions: [missedSession],
      sit_in: {
        sit_in_method: "physical",
        sit_in_course: {
          id: "course-sitin",
          code: "MATH201",
          name: "Mathematics Make-up",
          subject_code: "MATH",
          subject_name: "Mathematics",
        },
        available_sessions: sitInSessions,
      },
    },
  ],
};

const config = {
  form: {
    max_date_range_days: 30,
    require_reason: false,
    reason_categories: [
      { value: "not_feeling_well", label: "Not feeling well" },
      { value: "appointment", label: "Appointment" },
      { value: "school_activity", label: "School activity" },
      { value: "family_commitment", label: "Family commitment" },
      { value: "travel", label: "Travel" },
      { value: "other", label: "Other" },
    ],
    allow_free_text_reason: true,
    intro_text: "",
    confirmation_text: "Thank you for reporting.",
  },
  sit_in: {
    auto_resolve_enabled: true,
    zoom_description: "Zoom session.",
    max_sessions_per_absence: 10,
  },
  notifications: {
    sms_parent_enabled: true,
    sms_parent_template: "",
    sms_success_template: "",
    sms_special_approved_template: "",
    email_success_enabled: false,
    email_success_subject: "",
    email_success_body: "",
  },
  admin_contact: {
    email: "office@example.edu",
    phone: "+66 2123 4567",
    hours: "Mon-Fri 08:00-16:00",
  },
};

// The primary demo student (has a parent phone on file, needs a contact email).
const mainLookup = {
  wcode: "W250389",
  lookup_token: "lookup-token",
  email_input_required: true,
  parent_verification_available: true,
  nickname_hint: "J***",
  parent_phone_hint: "••••••5678",
};

// A second student with no phone on file → the enrollment flow.
const enrollLookup = {
  wcode: "W112233",
  lookup_token: "lookup-token-2",
  email_input_required: false,
  parent_verification_available: false,
  nickname_hint: "S***",
};

const verifiedWcodes = new Set(); // survives reloads while the worker lives
let configRequests = 0;

function maskPhone(raw) {
  const digits = (raw || "").replace(/\D/g, "");
  return "•".repeat(Math.max(0, digits.length - 4)) + digits.slice(-4);
}

function verificationPayload(wcode, phoneMask) {
  return {
    token: wcode === "W112233" ? "verification-token-2" : "verification-token",
    status: verifiedWcodes.has(wcode) ? "verified" : "pending",
    wcode,
    parent_phone: phoneMask || "••••••5678",
    delivery_status: "accepted",
    otp_last_sent_at: isoNow(Date.now() - 60_000),
    otp_code_expires_at: isoNow(Date.now() + 10 * 60_000),
    expires_at: isoNow(Date.now() + 60 * 60_000),
    verified_at: verifiedWcodes.has(wcode) ? isoNow(Date.now()) : undefined,
  };
}

const json = (data, status = 200, delay = 0) =>
  new Promise((resolve) =>
    setTimeout(() => {
      resolve(
        new Response(JSON.stringify(data), {
          status,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }, delay),
  );

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (url.origin !== self.location.origin || !url.pathname.startsWith("/api/")) {
    return; // let everything else through untouched
  }
  const method = (event.request.method || "GET").toUpperCase();
  const path = url.pathname;
  const delay = (ms) => new Promise((r) => setTimeout(r, ms));

  event.respondWith(
    (async () => {
      let body = {};
      if (method === "POST" || method === "PUT") {
        try {
          body = await event.request.clone().json();
        } catch {
          body = {};
        }
      }

      // Shell auth check: the public absence flow is reachable signed-out.
      if (path === "/api/v1/me" && method === "GET") {
        return json({ code: "unauthorized", message: "Not authenticated" }, 401);
      }

      if (path === "/api/v1/absence-form-config" && method === "GET") {
        const d = configRequests++ === 0 ? 1400 : 250; // first paint shows the loading state
        return json(config, 200, d);
      }

      if (path === "/api/v1/absence-self-service/lookup" && method === "POST") {
        await delay(900); // show the lookup spinner
        const wcode = String(body.wcode || "").toUpperCase().replace(/[^A-Z0-9]/g, "");
        if (wcode === "W250389") return json(mainLookup);
        if (wcode === "W112233") return json(enrollLookup);
        return json({ code: "student_not_found", message: "Student not found" }, 404);
      }

      if (path === "/api/v1/absence-self-service/me" && method === "GET") {
        return json({
          wcode: "W250389",
          display_name: "John",
          email_on_file: true,
          subjects: [{ id: "subject-math", code: "MATH", name: "Mathematics" }],
        });
      }

      if (path.startsWith("/api/v1/absence-self-service/sessions") && method === "GET") {
        await delay(700); // show the classes loading state
        return json(sessionsInRange);
      }

      if (path === "/api/v1/absences/parent-verification/send" && method === "POST") {
        const phoneMask = body.parent_phone ? maskPhone(body.parent_phone) : undefined;
        const wcode = mainLookup.wcode;
        return json(verificationPayload(wcode, phoneMask));
      }

      if (path === "/api/v1/absences/parent-verification/status" && method === "POST") {
        const wcode = body.token === "verification-token-2" ? "W112233" : "W250389";
        return json(verificationPayload(wcode));
      }

      if (path === "/api/v1/absences/parent-verification/verify" && method === "POST") {
        await delay(450);
        if (String(body.code || "").replace(/\D/g, "") === "123456") {
          const wcode = body.token === "verification-token-2" ? "W112233" : "W250389";
          verifiedWcodes.add(wcode);
          return json(verificationPayload(wcode));
        }
        return json({ code: "invalid_code", message: "That code isn't right." }, 400);
      }

      if (path === "/api/v1/absences/batch" && method === "POST") {
        await delay(1600); // show the submitting state on Review
        const reason = String(body.reason || "Not feeling well");
        return json({
          items: [
            {
              id: "absence-walkthrough",
              wcode: "W250389",
              status: "pending",
              version: 1,
              created_at: isoNow(Date.now()),
              updated_at: isoNow(Date.now()),
              student_name: "John Smith",
              subject_id: "subject-math",
              subject_code: "MATH",
              subject_name: "Mathematics",
              course_id: "course-math",
              course_code: "MATH101",
              course_name: "Mathematics 101",
              date_from: missedDate,
              date_to: missedDate,
              reason,
              sit_in_method: "physical",
              sit_in_course_id: "course-sitin",
              sit_in_course_code: "MATH201",
              sit_in_course_name: "Mathematics Make-up",
              sit_in_subject_name: "Mathematics",
              missed_sessions: [
                {
                  id: "missed-record",
                  session_id: missedSession.id,
                  course_id: "course-math",
                  course_code: "MATH101",
                  course_name: "Mathematics 101",
                  subject_name: "Mathematics",
                  start_at: missedSession.start_at,
                  end_at: missedSession.end_at,
                },
              ],
              sit_ins: [
                {
                  id: "sitin-record",
                  session_id: sitInSessions[0].id,
                  course_id: "course-sitin",
                  course_code: "MATH201",
                  course_name: "Mathematics Make-up",
                  subject_name: "Mathematics",
                  start_at: sitInSessions[0].start_at,
                  end_at: sitInSessions[0].end_at,
                },
              ],
            },
          ],
        });
      }

      // Unknown /api route: let it hit the (absent) backend proxy as before.
      return fetch(event.request);
    })(),
  );
});
