import { LegalPage, LegalSection } from '@/components/legal/legal-page';
import type { Operator } from '@/lib/legal';

export function TermsContent({ operator }: { operator: Operator }) {
  return (
    <LegalPage title="Terms of Service" other="privacy" operator={operator}>
      <LegalSection title="Who you are dealing with">
        <p>
          Steerpost is open-source software under the AGPL-3.0. These terms are between you and the operator of this copy (named under Contact below). The Steerpost
          maintainers do not run this instance and are not a party to these terms.
        </p>
      </LegalSection>

      <LegalSection title="What the service does">
        <p>
          Steerpost lets you write posts, schedule them and publish them to the networks you connect. You and the AI agents you give an API key can
          use it. Which networks work, and what each can do, is shown in the app. Some are not available yet and the app says so.
        </p>
      </LegalSection>

      <LegalSection title="Your account">
        <ul>
          <li>Give a real email address and keep your password and API keys secret. You are responsible for what is done with them, including by AI agents you connect.</li>
          <li>Dangerous actions by an API key need your approval, unless you made the key trusted. Approving one is your decision.</li>
        </ul>
      </LegalSection>

      <LegalSection title="Your content and the networks">
        <p>
          You own your content. You allow the operator&apos;s instance to store it and to send it to the networks you choose, when you or your
          agents ask. You must have the right to post it and you must follow the rules of each network. A network can reject a post, limit your
          account or disconnect it, and Steerpost cannot prevent that.
        </p>
      </LegalSection>

      <LegalSection title="What you may not do">
        <ul>
          <li>Break the law or the rules of a connected network, or post content you have no right to post.</li>
          <li>Send spam, or try to get around rate limits or access controls.</li>
          <li>Attack, overload or probe the instance, or use it to harm others.</li>
        </ul>
      </LegalSection>

      <LegalSection title="No guarantee">
        <p>
          The service is provided as is. Posts may be delayed, may fail, or in rare cases may be published twice if a network gives an unclear
          answer; the app marks such posts. The operator does not promise uptime, and is not liable for losses that follow from the service
          being unavailable or wrong, as far as the law allows.
        </p>
      </LegalSection>

      <LegalSection title="Ending">
        <p>
          You can stop using the service at any time. The operator can suspend or delete an account that breaks these terms or that puts the
          instance at risk. See the Privacy Policy for what happens to your data and how to ask for deletion.
        </p>
      </LegalSection>

      <LegalSection title="Changes">
        <p>The version above changes when the meaning of this text changes. Continued use after a change means you accept the new version.</p>
      </LegalSection>

      <LegalSection title="Contact">
        <p>Operator: {operator.name}</p>
        <p>Contact: {operator.contact}</p>
      </LegalSection>
    </LegalPage>
  );
}
