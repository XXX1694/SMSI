import { render, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Table, Tbody, Td, Th, Thead, Tr } from '@/components/ui/table';

function Sample() {
  return (
    <Table label="Publication attempts">
      <Thead>
        <Tr>
          <Th>Result</Th>
          <Th>Error</Th>
        </Tr>
      </Thead>
      <Tbody>
        <Tr>
          <Td label="Result">Failed</Td>
          <Td label="Error">Rate limit reached</Td>
          <Td>
            <button>Retry</button>
          </Td>
        </Tr>
      </Tbody>
    </Table>
  );
}

describe('Table', () => {
  it('is a labelled, keyboard-focusable scroll region around a real table', () => {
    render(<Sample />);
    const region = screen.getByRole('region', { name: 'Publication attempts' });
    expect(region).toHaveAttribute('tabindex', '0');
    expect(within(region).getByRole('table')).toBeInTheDocument();
    expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Result', 'Error']);
  });

  it('keeps every column title on its cell for the stacked mobile layout, and none on an actions cell', () => {
    render(<Sample />);
    const cells = screen.getAllByRole('cell');
    expect(cells.map((c) => c.getAttribute('data-label'))).toEqual(['Result', 'Error', null]);
    expect(cells[1]).toHaveTextContent('Rate limit reached');
  });
});
