import { LegalPage, LegalSection } from '@/components/legal/legal-page';
import type { Operator } from '@/lib/legal';

export function PrivacyContent({ operator }: { operator: Operator }) {
  return (
    <LegalPage title="Privacy Policy" other="terms" operator={operator}>
      <LegalSection title="Who this is about">
        <p>
          Steerpost is software that schedules and publishes social posts. Anyone can run their own copy. This policy describes what that
          software does with your data. The person or organisation that runs this copy is its operator (named under Contact below).
        </p>
        <p>The operator decides what happens to your data and answers for it. Steerpost does not run a central service that collects your data.</p>
      </LegalSection>

      <LegalSection title="What is stored">
        <ul>
          <li>Account: your email address, your display name and a salted hash of your password (Argon2id). The password itself is never stored.</li>
          <li>Terms: the version of these texts you accepted and when.</li>
          <li>Connected networks: the access tokens or credentials that LinkedIn, Telegram, Discord, Mastodon and Bluesky give Steerpost. They are encrypted at rest and never shown in the interface, logs or audit log.</li>
          <li>Content: your posts, their schedule and status, and the media you upload.</li>
          <li>Audit log: what was done, by you or by an AI agent using one of your API keys, and when. It holds no post text and no secrets.</li>
          <li>Approval requests: when an AI agent asks to publish, retry, delete or schedule soon, a copy of the post title and text it wants to act on is kept so you can review it. Decided and expired requests are deleted after a retention period (30 days by default).</li>
          <li>Sessions: a hash of each session, the browser user-agent string (up to 256 characters) and the IP address it came from. API keys: a hash of each key and when it was last used. Key secrets are shown once and stored only as hashes.</li>
          <li>IP addresses: used to rate-limit requests and kept in sessions and the audit log for security.</li>
        </ul>
      </LegalSection>

      <LegalSection title="Where it is stored">
        <p>
          On the operator&apos;s server: a PostgreSQL database and Redis for queues. Media goes to S3-compatible object storage that the operator
          chooses (for example MinIO on the same server, or a cloud provider). Ask the operator where this copy runs.
        </p>
      </LegalSection>

      <LegalSection title="What leaves the server">
        <p>
          Steerpost calls the APIs of the networks you connect, only on your behalf: LinkedIn, Telegram, Discord, Mastodon and Bluesky. It sends
          the post text and media you chose to publish and reads back the status and ids it needs. Those networks have their own privacy
          policies, which apply to what they receive.
        </p>
        <p>If the operator turned on email, a mail server also receives your address to send verification and password-reset messages.</p>
        <p>There are no advertising or analytics trackers, and no third-party scripts, in the web app. It sets two cookies, a session cookie and a CSRF cookie, which are needed to sign in.</p>
      </LegalSection>

      <LegalSection title="How long it is kept">
        <ul>
          <li>Posts, media and connected networks: until you delete them or the account.</li>
          <li>Sessions: until they expire (7 days by default) or you sign out.</li>
          <li>Audit log: kept with the account. There is no automatic expiry yet.</li>
          <li>Backups: whatever the operator keeps. Ask them how long.</li>
        </ul>
      </LegalSection>

      <LegalSection title="Export and deletion">
        <p>
          Today: you can delete single posts and media and disconnect a network in the app. Disconnecting removes the stored credentials.
          Self-service export of all your data and self-service account deletion do not exist yet (they are listed in the project roadmap).
        </p>
        <p>Until they do, ask the operator at the contact address below to send you your data or to delete your account.</p>
      </LegalSection>

      <LegalSection title="Your rights">
        <p>
          Depending on where you live you may have rights to access, correct, export or delete your data and to complain to a regulator. Use
          the contact address below. The operator is the controller of your data.
        </p>
      </LegalSection>

      <LegalSection title="Changes">
        <p>
          The version above changes when the meaning of this text changes. New accounts accept the current version when they register. Existing
          accounts are not blocked by a new version.
        </p>
      </LegalSection>

      <LegalSection title="Contact">
        <p>Operator: {operator.name}</p>
        <p>Contact: {operator.contact}</p>
      </LegalSection>
    </LegalPage>
  );
}
