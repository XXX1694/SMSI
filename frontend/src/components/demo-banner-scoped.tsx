'use client';
import { DemoBanner } from '@/components/demo-banner';
import { DemoScope } from '@/i18n/scopes/demo';

/** The demo banner with the `shell` messages it reads. Only the demo build imports this module (see DemoBannerSlot). */
export default function DemoBannerScoped() {
  return (
    <DemoScope>
      <DemoBanner />
    </DemoScope>
  );
}
