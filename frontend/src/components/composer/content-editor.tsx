'use client';
import { Button } from '@/components/ui/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/input';
import { effectiveContent } from '@/lib/composer';
import { providerLabel } from '@/lib/normalize';
import type { Provider, SocialAccount } from '@/lib/types';
import { CharCounter } from './char-counter';

interface Props {
  content: string;
  overrides: Record<string, string>;
  selected: SocialAccount[];
  providers: Provider[];
  onContent: (v: string) => void;
  onOverride: (accountId: string, v: string) => void;
}

export function ContentEditor({ content, overrides, selected, providers, onContent, onOverride }: Props) {
  const maxFor = (a: SocialAccount) => providers.find((p) => p.id === a.provider)?.capabilities.maxTextLength ?? 0;
  const strictest = selected.length ? Math.min(...selected.map(maxFor).filter((n) => n > 0), Number.MAX_SAFE_INTEGER) : 0;
  const strictMax = strictest === Number.MAX_SAFE_INTEGER ? 0 : strictest;

  return (
    <Tabs defaultValue="all">
      <TabsList aria-label="Content scope">
        <TabsTrigger value="all">All platforms</TabsTrigger>
        {selected.map((a) => (
          <TabsTrigger key={a.id} value={a.id}>
            {providerLabel(a.provider)}
            <span className="ml-1 max-w-[10rem] truncate text-muted-foreground">{a.display_name}</span>
            {overrides[a.id]?.trim() ? ' •' : ''}
            <span className="sr-only">{overrides[a.id]?.trim() ? ' (customised)' : ''}</span>
          </TabsTrigger>
        ))}
      </TabsList>
      <TabsContent value="all" className="space-y-1.5">
        <Textarea
          id="composer-content"
          aria-label="Post content"
          rows={8}
          placeholder="What do you want to share?"
          value={content}
          onChange={(e) => onContent(e.target.value)}
        />
        <div className="flex justify-between gap-3 text-xs text-muted-foreground">
          <span>Used for every selected account unless customised in its tab.</span>
          <CharCounter text={content} max={strictMax} label="Universal content" />
        </div>
      </TabsContent>
      {selected.map((a) => {
        const text = effectiveContent({ content, overrides }, a.id);
        const customised = Boolean(overrides[a.id]?.trim());
        return (
          <TabsContent key={a.id} value={a.id} className="space-y-1.5">
            <Textarea
              aria-label={`${providerLabel(a.provider)} content for ${a.display_name}`}
              rows={8}
              placeholder={content || 'Customise the text for this account'}
              value={overrides[a.id] ?? ''}
              onChange={(e) => onOverride(a.id, e.target.value)}
            />
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs text-muted-foreground">
                {customised ? 'Custom text for this account.' : 'Empty: the universal content is used.'}
              </span>
              <div className="flex items-center gap-3">
                {customised ? (
                  <Button variant="link" size="sm" className="h-auto p-0" onClick={() => onOverride(a.id, '')}>
                    Reset
                  </Button>
                ) : null}
                <CharCounter text={text} max={maxFor(a)} label={`${providerLabel(a.provider)} content`} />
              </div>
            </div>
          </TabsContent>
        );
      })}
    </Tabs>
  );
}
