/**
 * Minimal GAS webhook — receives email payload, sends via Gmail.
 *
 * All business logic, template rendering, and recipient management
 * happens on our Go backend. This script is a thin relay.
 *
 * Deploy: script.google.com → Deploy → Web app
 *   Execute as: Me
 *   Who has access: Anyone
 *
 * GAS always responds HTTP 200. Check JSON body for success/failure.
 */

function doPost(e) {
  try {
    var payload = JSON.parse(e.postData.contents);
    var to = payload.to;
    var subject = payload.subject;
    var body = payload.body;

    if (!to || !subject || !body) {
      return respond(false, "Missing required field(s): to, subject, body");
    }

    MailApp.sendEmail({
      to: to,
      subject: subject,
      body: stripHtml(body),
      htmlBody: body
    });
    Logger.log("Sent to: " + to);
    return respond(true, null);

  } catch (err) {
    Logger.log("Error: " + err.message);
    return respond(false, err.message);
  }
}

function respond(success, error) {
  return ContentService
    .createTextOutput(JSON.stringify({ success: success, error: error }))
    .setMimeType(ContentService.MimeType.JSON);
}

function stripHtml(html) {
  return String(html)
    .replace(/<br\s*\/?>/gi, "\n")
    .replace(/<\/p>/gi, "\n")
    .replace(/<[^>]*>/g, "")
    .replace(/&nbsp;/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&#34;/g, '"')
    .replace(/&#39;/g, "'");
}
