import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuditView } from '@/components/developer/audit-view';
import type { AuditLog } from '@/lib/types';

const apiMock = vi.hoisted(() => ({ audit: { list: vi.fn() } }));
vi.mock('@/lib/api', () => ({ api: apiMock, ApiError: class ApiError extends Error {} }));
vi.mock('@/components/prefs-provider', () => ({ usePrefs: () => ({ timezone: 'UTC' }) }));

const row = (over: Partial<AuditLog>): AuditLog => ({
  id: 'id-' + Math.random(),
  actor_type: 'user',
  actor_label: 'Demo User',
  action: 'user.login',
  resource_type: 'user',
  resource_id: null,
  request_id: null,
  ip: null,
  created_at: '2026-10-07T12:00:00Z',
  ...over,
});
const toolCall = (tool: string, status: number, errorCode?: string, viaGateway = true): AuditLog =>
  row({
    actor_type: 'api_key',
    actor_label: 'MCP: Claude Desktop',
    action: 'mcp.tool_call',
    resource_type: 'api_key',
    metadata: { tool, status, via_gateway: viaGateway, route: '/api/v1/posts', ...(errorCode ? { error_code: errorCode } : {}) },
  });

const page = (items: AuditLog[]) => ({ items, next_cursor: null });

beforeEach(() => apiMock.audit.list.mockReset());

describe('AuditView', () => {
  it('lists everything by default and shows agent tool calls with tool, status and label', async () => {
    apiMock.audit.list.mockResolvedValue(page([row({}), toolCall('list_posts', 200), toolCall('publish_post', 403, 'INSUFFICIENT_SCOPE')]));
    render(<AuditView />);
    expect(await screen.findByText('list_posts')).toBeInTheDocument();
    expect(screen.getByText('user.login')).toBeInTheDocument();
    expect(screen.getByText('publish_post')).toBeInTheDocument();
    expect(screen.getByText('200')).toBeInTheDocument();
    expect(screen.getByText('403 INSUFFICIENT_SCOPE')).toBeInTheDocument();
    expect(screen.getAllByText(/MCP: Claude Desktop/)).toHaveLength(2);
    expect(apiMock.audit.list).toHaveBeenCalledWith(25, undefined, undefined);
  });

  it('"Agent actions" asks the API for mcp.tool_call only and can switch back', async () => {
    apiMock.audit.list.mockResolvedValueOnce(page([row({})])).mockResolvedValueOnce(page([toolCall('get_post', 200)]));
    render(<AuditView />);
    await screen.findByText('user.login');
    await userEvent.click(screen.getByRole('button', { name: 'Agent actions' }));
    expect(await screen.findByText('get_post')).toBeInTheDocument();
    expect(screen.queryByText('user.login')).not.toBeInTheDocument();
    expect(apiMock.audit.list).toHaveBeenLastCalledWith(25, undefined, 'mcp.tool_call');
    expect(screen.getByRole('button', { name: 'Agent actions' })).toHaveAttribute('aria-pressed', 'true');

    apiMock.audit.list.mockResolvedValueOnce(page([row({})]));
    await userEvent.click(screen.getByRole('button', { name: 'All activity' }));
    await waitFor(() => expect(apiMock.audit.list).toHaveBeenLastCalledWith(25, undefined, undefined));
  });

  it('has a distinct empty state for agent actions', async () => {
    apiMock.audit.list.mockResolvedValueOnce(page([row({})])).mockResolvedValueOnce(page([]));
    render(<AuditView />);
    await screen.findByText('user.login');
    await userEvent.click(screen.getByRole('button', { name: 'Agent actions' }));
    expect(await screen.findByText('No agent actions yet')).toBeInTheDocument();
  });

  it('keeps the filter available when loading fails, and retries', async () => {
    apiMock.audit.list.mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce(page([toolCall('list_posts', 200)]));
    render(<AuditView />);
    expect(await screen.findByRole('alert')).toHaveTextContent('boom');
    expect(screen.getByRole('button', { name: 'Agent actions' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByText('list_posts')).toBeInTheDocument();
  });

  it('marks tool calls that did not come through the MCP gateway as Direct API', async () => {
    apiMock.audit.list.mockResolvedValue(page([toolCall('list_posts', 200), toolCall('get_post', 200, undefined, false)]));
    render(<AuditView />);
    await screen.findByText('get_post');
    expect(screen.getAllByText('Direct API')).toHaveLength(1);
  });

  it('tolerates tool rows without metadata', async () => {
    apiMock.audit.list.mockResolvedValue(page([row({ action: 'mcp.tool_call', actor_type: 'api_key', metadata: undefined })]));
    render(<AuditView />);
    expect(await screen.findByText('unknown tool')).toBeInTheDocument();
  });
});
