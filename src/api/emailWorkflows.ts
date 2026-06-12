import { apiJson } from './client';
import type { EmailWorkflow } from '../types';

export async function listWorkflows(): Promise<EmailWorkflow[]> {
  return apiJson<EmailWorkflow[]>('/api/v1/admin/email-workflows');
}

export async function getWorkflow(id: string): Promise<EmailWorkflow> {
  return apiJson<EmailWorkflow>(`/api/v1/admin/email-workflows/${id}`);
}

export async function createWorkflow(data: {
  name: string;
  template_id: string;
  enabled?: boolean;
  trigger_description?: string;
  recipients?: string[];
}): Promise<EmailWorkflow> {
  return apiJson<EmailWorkflow>('/api/v1/admin/email-workflows', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function updateWorkflow(id: string, data: Partial<EmailWorkflow>): Promise<EmailWorkflow> {
  return apiJson<EmailWorkflow>(`/api/v1/admin/email-workflows/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function deleteWorkflow(id: string): Promise<void> {
  await apiJson<void>(`/api/v1/admin/email-workflows/${id}`, { method: 'DELETE' });
}

export async function sendWorkflow(id: string): Promise<{ sent: number; failed: number; skipped: number; message: string }> {
  return apiJson<{ sent: number; failed: number; skipped: number; message: string }>(`/api/v1/admin/email-workflows/${id}/send`, { method: 'POST' });
}

export async function sendAllWorkflows(): Promise<{ sent: number; failed: number; skipped: number; message: string }> {
  return apiJson<{ sent: number; failed: number; skipped: number; message: string }>('/api/v1/admin/email-workflows/send-all', { method: 'POST' });
}
