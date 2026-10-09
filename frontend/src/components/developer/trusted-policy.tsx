'use client';
import { Notice } from '@/components/states';
import { Checkbox } from '@/components/ui/checkbox';

interface Props {
  trusted: boolean;
  confirmed: boolean;
  onTrusted: (v: boolean) => void;
  onConfirmed: (v: boolean) => void;
}

/**
 * Opt-out of the owner's approval for this key (dangerous_policy "trusted", D-013). Off by default, and turning it on
 * needs a second, explicit confirmation: after that the key can publish, delete and disconnect without asking.
 */
export function TrustedPolicyField({ trusted, confirmed, onTrusted, onConfirmed }: Props) {
  return (
    <div className="space-y-3">
      <div className="flex items-start gap-2.5">
        <Checkbox id="key-trusted" checked={trusted} onCheckedChange={(c) => onTrusted(c === true)} />
        <label htmlFor="key-trusted" className="text-sm">
          <span className="font-medium">Trusted key: do not ask me before dangerous actions.</span>
          <span className="block text-muted-foreground">
            By default, dangerous actions by this key wait for your approval in Approvals.
          </span>
        </label>
      </div>
      {trusted ? (
        <>
          <Notice tone="danger">
            A trusted key acts without asking. If it leaks, or the agent misbehaves, it can publish to your live networks, delete posts and disconnect
            accounts straight away. Leave this off unless the key runs unattended.
          </Notice>
          <div className="flex items-start gap-2.5">
            <Checkbox id="key-trusted-ack" checked={confirmed} onCheckedChange={(c) => onConfirmed(c === true)} />
            <label htmlFor="key-trusted-ack" className="text-sm">
              I understand this key can publish, delete and disconnect without my approval.
            </label>
          </div>
        </>
      ) : null}
    </div>
  );
}
