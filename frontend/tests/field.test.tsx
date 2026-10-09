import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Button } from '@/components/ui/button';
import { Card, Section } from '@/components/ui/card';
import { CheckboxField } from '@/components/ui/checkbox';
import { Field, Input, Select, Textarea } from '@/components/ui/input';
import { SecretInput } from '@/components/ui/secret-input';

describe('Field', () => {
  it('links the label, hint and control without any ids from the caller', () => {
    render(
      <Field label="Name" hint="Shown in the list.">
        <Input />
      </Field>,
    );
    const input = screen.getByLabelText('Name');
    expect(input).toHaveAttribute('aria-describedby', screen.getByText('Shown in the list.').id);
    expect(input).not.toHaveAttribute('aria-invalid');
  });

  it('gives two fields different ids', () => {
    render(
      <>
        <Field label="First">
          <Input />
        </Field>
        <Field label="Second">
          <Input />
        </Field>
      </>,
    );
    expect(screen.getByLabelText('First').id).not.toBe(screen.getByLabelText('Second').id);
  });

  it('marks the control invalid and describes it by the error instead of the hint', () => {
    render(
      <Field label="Email" hint="We never share it." error="Enter a valid email." required>
        <Input />
      </Field>,
    );
    const input = screen.getByLabelText('Email');
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAttribute('aria-required', 'true');
    expect(input).toHaveAttribute('aria-describedby', screen.getByRole('alert').id);
    expect(screen.getByRole('alert')).toHaveTextContent('Enter a valid email.');
    expect(screen.queryByText('We never share it.')).not.toBeInTheDocument();
  });

  it('wires Textarea, Select and SecretInput the same way', () => {
    render(
      <>
        <Field label="Bio" error="Too long.">
          <Textarea />
        </Field>
        <Field label="Zone" hint="UTC is safe.">
          <Select>
            <option>UTC</option>
          </Select>
        </Field>
        <Field label="Token" error="Rejected.">
          <SecretInput label="Token" />
        </Field>
      </>,
    );
    expect(screen.getByLabelText('Bio')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByLabelText('Zone')).toHaveAttribute('aria-describedby');
    expect(screen.getByLabelText('Token')).toHaveAttribute('aria-invalid', 'true');
  });

  it('keeps the legacy htmlFor id and lets explicit props win', () => {
    render(
      <Field label="Mine" htmlFor="custom-id" hint="Help">
        <Input aria-describedby="elsewhere" />
      </Field>,
    );
    const input = screen.getByLabelText('Mine');
    expect(input.id).toBe('custom-id');
    expect(input).toHaveAttribute('aria-describedby', 'elsewhere');
  });

  it('appends "(optional)" to the label', () => {
    render(
      <Field label="Title" optional>
        <Input />
      </Field>,
    );
    expect(screen.getByLabelText('Title (optional)')).toBeInTheDocument();
  });

  it('leaves a control outside a Field untouched', () => {
    render(<Input aria-label="Loose" />);
    expect(screen.getByLabelText('Loose')).not.toHaveAttribute('aria-describedby');
  });
});

describe('CheckboxField', () => {
  it('toggles from its label and describes itself', async () => {
    const onChange = vi.fn();
    render(<CheckboxField label="Also revoke keys" description="They keep working otherwise." onCheckedChange={onChange} />);
    const box = screen.getByRole('checkbox', { name: 'Also revoke keys' });
    expect(box).toHaveAccessibleDescription('They keep working otherwise.');
    await userEvent.click(screen.getByText('Also revoke keys'));
    expect(onChange).toHaveBeenCalledWith(true);
  });
});

describe('Button loading', () => {
  it('disables the button and sets aria-busy', async () => {
    const onClick = vi.fn();
    render(
      <Button loading onClick={onClick}>
        Saving…
      </Button>,
    );
    const btn = screen.getByRole('button', { name: 'Saving…' });
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute('aria-busy', 'true');
    await userEvent.click(btn);
    expect(onClick).not.toHaveBeenCalled();
  });

  it('is enabled and not busy by default', () => {
    render(<Button>Go</Button>);
    expect(screen.getByRole('button', { name: 'Go' })).not.toHaveAttribute('aria-busy');
    expect(screen.getByRole('button', { name: 'Go' })).toBeEnabled();
  });
});

describe('Card and Section', () => {
  it('renders a Card as the requested element', () => {
    render(
      <ul>
        <Card as="li">Row</Card>
      </ul>,
    );
    expect(screen.getByRole('listitem')).toHaveTextContent('Row');
  });

  it('titles a Section with a heading and an action', () => {
    render(
      <Section title="Profile" action={<button>Edit</button>}>
        body
      </Section>,
    );
    expect(screen.getByRole('heading', { level: 2, name: 'Profile' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument();
  });
});
