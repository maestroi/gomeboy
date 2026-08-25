{#if $toast}
	<div class="toast toast-{$toast.kind}" transition:fade={{duration: 150}}>
		{$toast.text}
	</div>
{/if}

<script>
	import Game from "$lib/game";
	import { fade } from "svelte/transition";
	let { toast } = Game;
</script>

<style lang="scss">
	.toast {
		position: absolute;
		top: 8px;
		left: 50%;
		transform: translateX(-50%);
		padding: 4px 12px;
		border-radius: 4px;
		background: rgba(0, 0, 0, 0.75);
		color: #fff;
		font-size: 13px;
		pointer-events: none;
		z-index: 10;
	}

	.toast-error {
		background: rgba(180, 30, 30, 0.85);
	}
</style>
