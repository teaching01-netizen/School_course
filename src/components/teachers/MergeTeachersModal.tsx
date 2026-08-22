import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useToast } from '../../hooks/useToast';
import { useApiQuery } from '@/hooks/useApiQuery';
import { useApiMutation } from '@/hooks/useApiMutation';
import Modal from '../Modal';
import Button from '../ui/Button';
import TypeaheadSelect from '../TypeaheadSelect';
import LoadingSkeleton from '../ui/LoadingSkeleton';

export type MergeTeacherRow = { id: string; username: string; full_name: string | null };

type MergeAccount = {
  id: string;
  username: string;
  full_name: string;
  email: string;
  role: string;
  deleted: boolean;
  created_at: string;
  is_legacy: boolean;
};

type MergeImpact = {
  sessions_live: number;
  sessions_deleted: number;
  courses: number;
  series: number;
  course_teacher_rows: number;
  availability_blocks: number;
  external_ref_mappings: number;
  conflict_sessions: number;
};

type MergePreview = { duplicate: MergeAccount; canonical: MergeAccount; impact: MergeImpact };
type MergeResult = { impact: MergeImpact; canonical: MergeAccount };

type Step = 'pick' | 'preview' | 'done';

const displayName = (t: { username: string; full_name: string | null }) => t.full_name?.trim() || t.username;

function AccountCard({ account, hint, tone }: { account: MergeAccount | MergeTeacherRow; hint: string; tone: 'duplicate' | 'canonical' }) {
  const email = 'email' in account ? account.email : '';
  return (
    <div
      className={`rounded-sm border px-3 py-2 ${
        tone === 'duplicate'
          ? 'border-wi-line bg-[var(--color-wi-callout)]'
          : 'border-[var(--color-wi-primary)] bg-white'
      }`}
    >
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-sm font-medium text-[var(--color-wi-text)]">
          {displayName(account)}
        </span>
        <span className="font-mono text-xs text-[var(--color-wi-text-light)]">{account.username}</span>
      </div>
      <p className="mt-0.5 text-xs text-[var(--color-wi-faint)]">
        {email ? `${email} · ` : ''}
        {hint}
      </p>
    </div>
  );
}

function ImpactRow({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-0.5">
      <span className="text-sm text-[var(--color-wi-text-light)]">{label}</span>
      <span className="font-mono text-sm text-[var(--color-wi-text)] tabular-nums">{value}</span>
    </div>
  );
}

const totalMoved = (i: MergeImpact) =>
  i.sessions_live + i.sessions_deleted + i.courses + i.series + i.course_teacher_rows + i.availability_blocks + i.external_ref_mappings;

export default function MergeTeachersModal({
  open,
  duplicate,
  teachers,
  onClose,
  onMerged,
}: {
  open: boolean;
  duplicate: MergeTeacherRow | null;
  teachers: MergeTeacherRow[];
  onClose: () => void;
  onMerged: () => void;
}) {
  const { addToast } = useToast();
  const [step, setStep] = useState<Step>('pick');
  const [canonicalId, setCanonicalId] = useState('');
  const [result, setResult] = useState<MergeResult | null>(null);
  const backRef = useRef<HTMLButtonElement>(null);

  const canonical = teachers.find((t) => t.id === canonicalId) ?? null;
  const previewUrl =
    open && duplicate && canonicalId && duplicate.id !== canonicalId
      ? `/api/v1/teacher/merge/preview?duplicate_user_id=${duplicate.id}&canonical_user_id=${canonicalId}`
      : null;
  const { data: preview, loading: previewLoading, error: previewError, refetch: refetchPreview } = useApiQuery<MergePreview>(previewUrl, [previewUrl]);
  const { mutate: merge, loading: merging } = useApiMutation<{ duplicate_user_id: string; canonical_user_id: string }, MergeResult>('POST', {
    invalidate: [['reference', '/api/v1/users?role=Teacher']],
  });

  // Reset the flow whenever the modal opens for a (different) duplicate.
  useEffect(() => {
    if (open) return;
    setStep('pick');
    setCanonicalId('');
    setResult(null);
  }, [open]);

  // The consequential action is guarded by landing focus on the safe path.
  useEffect(() => {
    if (step === 'preview' && !previewLoading) backRef.current?.focus();
  }, [step, previewLoading]);

  if (!open || !duplicate) return null;

  const options = teachers
    .filter((t) => t.id !== duplicate.id && !t.username.startsWith('legacy:'))
    .map((t) => ({ value: t.id, label: `${displayName(t)} (${t.username})`, keywords: t.username }));

  const onMerge = async () => {
    if (!canonicalId || merging) return;
    try {
      const res = await merge({ duplicate_user_id: duplicate.id, canonical_user_id: canonicalId }, '/api/v1/teacher/merge');
      setResult(res);
      setStep('done');
      addToast('success', `Teachers merged — ${duplicate.username} → ${res.canonical.username}`);
      onMerged();
    } catch (err) {
      addToast('error', err instanceof Error ? err.message : 'Merge failed');
    }
  };

  const previewCanonical = preview?.canonical;
  const confirmLabel = previewCanonical
    ? totalMoved(preview.impact) === 0
      ? 'Deactivate duplicate'
      : `Merge into ${displayName(previewCanonical)}`
    : 'Merge teachers';

  return (
    <Modal
      title="Merge duplicate teacher"
      onClose={onClose}
      size="lg"
      footer={
        step === 'pick' ? (
          <>
            <Button variant="secondary" onClick={onClose}>Cancel</Button>
            <Button variant="primary" disabled={!canonicalId} onClick={() => setStep('preview')}>
              Preview merge →
            </Button>
          </>
        ) : step === 'preview' ? (
          <>
            <Button ref={backRef} variant="secondary" onClick={() => setStep('pick')} disabled={merging}>← Back</Button>
            <Button variant="primary" onClick={() => void onMerge()} loading={merging} disabled={previewLoading || !!previewError}>
              {confirmLabel}
            </Button>
          </>
        ) : (
          <Button variant="primary" onClick={onClose}>Done</Button>
        )
      }
    >
      {step === 'pick' && (
        <div className="space-y-4">
          <p className="text-sm text-[var(--color-wi-text-light)]">
            Combine a legacy-sync duplicate into the real account. The teacher keeps one login and sees all
            their courses and student absences.
          </p>
          <div>
            <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-faint)]">Duplicate — from old site</p>
            <AccountCard account={duplicate} hint="cannot log in" tone="duplicate" />
          </div>
          <div className="flex justify-center" aria-hidden="true">
            <span className="text-[var(--color-wi-faint)]">▼ merge into</span>
          </div>
          <div>
            <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-faint)]">Keep this account</p>
            <TypeaheadSelect
              value={canonicalId}
              onChange={setCanonicalId}
              options={options}
              placeholder="Search teachers…"
            />
            {canonical && (
              <div className="mt-2">
                <AccountCard account={canonical} hint="active login" tone="canonical" />
              </div>
            )}
          </div>
        </div>
      )}

      {step === 'preview' && (
        <div className="space-y-4">
          {previewLoading ? (
            <LoadingSkeleton type="text" lines={6} />
          ) : previewError ? (
            <div className="space-y-3">
              <p className="text-sm text-[var(--color-wi-text)]">
                {previewError.message || 'Could not load the merge preview.'}
              </p>
              <Button variant="secondary" size="sm" onClick={() => void refetchPreview()}>Retry</Button>
            </div>
          ) : preview ? (
            <>
              <p className="text-sm text-[var(--color-wi-text-light)]">
                <span className="font-mono text-xs">{preview.duplicate.username}</span> ({displayName(preview.duplicate)}) →{' '}
                <span className="font-mono text-xs">{preview.canonical.username}</span> ({displayName(preview.canonical)})
              </p>
              <div className="rounded-sm border border-wi-line px-3 py-2">
                <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-faint)]">
                  Will be reassigned to {displayName(preview.canonical)}
                </p>
                <ImpactRow label="Sessions" value={preview.impact.sessions_live} />
                <ImpactRow label="Past (deleted) sessions" value={preview.impact.sessions_deleted} />
                <ImpactRow label="Courses" value={preview.impact.courses} />
                <ImpactRow label="Series" value={preview.impact.series} />
                <ImpactRow label="Availability blocks" value={preview.impact.availability_blocks} />
                <ImpactRow label="Legacy identity mappings" value={preview.impact.external_ref_mappings} />
              </div>
              {preview.impact.conflict_sessions > 0 && (
                <div className="rounded-sm border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900">
                  <span className="font-semibold">{preview.impact.conflict_sessions} session conflicts.</span>{' '}
                  These sessions overlap {displayName(preview.canonical)}&rsquo;s existing schedule or availability.
                  They still move — flagged as legacy conflicts, the same treatment as other imported overlaps.
                </div>
              )}
              <div className="rounded-sm border border-wi-line bg-[var(--color-wi-callout)] px-3 py-2">
                <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-wi-faint)]">After the merge</p>
                <ul className="list-disc space-y-0.5 pl-4 text-sm text-[var(--color-wi-text-light)]">
                  <li>{preview.duplicate.username} is deactivated and leaves all teacher lists.</li>
                  <li>{preview.canonical.username}&rsquo;s username, password and email don&rsquo;t change.</li>
                  <li>Students see no change in the absence form.</li>
                </ul>
              </div>
            </>
          ) : null}
        </div>
      )}

      {step === 'done' && result && (
        <div className="space-y-3">
          <p className="text-sm text-[var(--color-wi-text)]">
            <span className="font-semibold">✓ Merged {duplicate.username} into {result.canonical.username}.</span>
          </p>
          <p className="text-sm text-[var(--color-wi-text-light)]">
            {displayName(result.canonical)} now sees {result.impact.sessions_live} sessions, {result.impact.courses} courses
            and all their student absences on login.
          </p>
          <Link
            to={`/absences/dashboard?teacher_id=${result.canonical.id}`}
            className="inline-block px-3 py-1 text-sm rounded-sm bg-[var(--color-wi-primary)] hover:bg-[var(--color-wi-primary-dark)] text-white"
          >
            View teacher dashboard
          </Link>
        </div>
      )}
    </Modal>
  );
}
