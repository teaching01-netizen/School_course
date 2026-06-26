export type EmailTemplate = {
  id: string;
  name: string;
  subject: string;
  body: string;
  built_in: boolean;
  created_at: string;
  updated_at: string;
};

export type EmailWorkflow = {
  id: string;
  name: string;
  enabled: boolean;
  template_id: string;
  template_name: string;
  trigger_description: string;
  recipients: string[];
  last_sent_at?: string;
  last_sent_count: number;
  created_at: string;
  updated_at: string;
};
