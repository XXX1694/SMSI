'use client';
import { Field, Input } from '@/components/ui/input';

interface Props {
  date: string;
  time: string;
  timezone: string;
  onDate: (v: string) => void;
  onTime: (v: string) => void;
  /** Extra line for edit mode, e.g. how an unchanged time behaves. */
  note?: string;
}

export function ScheduleFields({ date, time, timezone, onDate, onTime, note }: Props) {
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-semibold">Schedule</legend>
      <div className="flex flex-wrap items-end gap-3">
        <Field label="Date" htmlFor="sched-date">
          <Input id="sched-date" type="date" value={date} onChange={(e) => onDate(e.target.value)} className="w-40" />
        </Field>
        <Field label="Time" htmlFor="sched-time">
          <Input id="sched-time" type="time" value={time} onChange={(e) => onTime(e.target.value)} className="w-32" />
        </Field>
      </div>
      <p className="text-xs text-muted-foreground">
        Time zone: {timezone}. Change it in Settings.
        {note ? ` ${note}` : ''}
      </p>
    </fieldset>
  );
}
