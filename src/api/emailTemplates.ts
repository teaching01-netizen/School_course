import { apiJson } from './client';
import type { EmailTemplate } from '../types';

export async function listTemplates(): Promise<EmailTemplate[]> {
  return apiJson<EmailTemplate[]>('/api/v1/admin/email-templates');
}

export async function getTemplate(id: string): Promise<EmailTemplate> {
  return apiJson<EmailTemplate>(`/api/v1/admin/email-templates/${id}`);
}

export async function createTemplate(data: { name: string; subject: string; body: string }): Promise<EmailTemplate> {
  return apiJson<EmailTemplate>('/api/v1/admin/email-templates', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function updateTemplate(id: string, data: { name?: string; subject?: string; body?: string }): Promise<EmailTemplate> {
  return apiJson<EmailTemplate>(`/api/v1/admin/email-templates/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function deleteTemplate(id: string): Promise<void> {
  await apiJson<void>(`/api/v1/admin/email-templates/${id}`, { method: 'DELETE' });
}

export async function previewTemplate(subject: string, body: string): Promise<{ subject: string; body: string }> {
  return apiJson<{ subject: string; body: string }>('/api/v1/admin/email-templates/preview', {
    method: 'POST',
    body: JSON.stringify({ subject, body }),
  });
}
