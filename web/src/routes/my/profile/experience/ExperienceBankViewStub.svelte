<script lang="ts">
  // Test-only: `ExperienceBankView` itself mounts `ExperienceAssistantPanel` (which boots a
  // real conversation via `onMount`) and fetches the bank on mount — none of that is what
  // page.spec.ts exercises. This stub exposes just enough to trigger the page's own
  // `onBankMutated` handler, the way a real bank edit would.
  let { onBankMutated }: { onBankMutated?: () => void } = $props();
</script>

<button type="button" onclick={() => onBankMutated?.()}>trigger bank mutated</button>
