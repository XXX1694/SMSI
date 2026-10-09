/** Version of the Terms and the Privacy Policy. The backend stores its own copy (backend/internal/domain/terms) and a Go test keeps them equal. */
export const LEGAL_VERSION = '2026-10-09';
/** The day the current texts took effect. */
export const LEGAL_EFFECTIVE_DATE = '2026-10-09';

export const OPERATOR_NAME_PLACEHOLDER = 'the operator of this Steerpost instance (name not set; the operator sets OPERATOR_NAME)';
export const OPERATOR_CONTACT_PLACEHOLDER = 'not set; the operator sets OPERATOR_CONTACT';

export type Operator = { name: string; contact: string; configured: boolean };

/**
 * Who runs this instance. Read from the frontend container's environment at request time (not at build time), because
 * the published image is shared by every operator. Without it the pages say plainly that the operator has not set it.
 */
export function readOperator(env: Record<string, string | undefined> = process.env): Operator {
  const name = env.OPERATOR_NAME?.trim() ?? '';
  const contact = env.OPERATOR_CONTACT?.trim() ?? '';
  return {
    name: name || OPERATOR_NAME_PLACEHOLDER,
    contact: contact || OPERATOR_CONTACT_PLACEHOLDER,
    configured: Boolean(name && contact),
  };
}
