import { LoadingRows } from '@/components/states';

/** Shown while a route of the app loads. It also makes the dynamic routes prefetchable (the layout reads a cookie). */
export default function Loading() {
  return <LoadingRows rows={4} />;
}
