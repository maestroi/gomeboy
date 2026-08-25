<div>
	<Scaler height=70 width=70>
		<DPad/>
	</Scaler>
	<Scaler height=70 width=70>
		<nav class="pad" style="transform-origin: top left;">
			<GameButton button={Button.A}/>
			<GameButton button={Button.B}/>
		</nav>
	</Scaler>
	<div class="save-load">
		<button on:click={() => Game.saveState()}>Save</button>
		<button on:click={() => Game.loadState()}>Load</button>
	</div>
	<div class="speed">
		<button class:active={$speed === 1} on:click={() => Game.setSpeed(1)}>1x</button>
		<button class:active={$speed === 2} on:click={() => Game.setSpeed(2)}>2x</button>
		<button class:active={$speed === 4} on:click={() => Game.setSpeed(4)}>4x</button>
	</div>
</div>

	<script>
	import Game from "$lib/game";
	let {speed} = Game;
	import {Button} from "$lib/consts";
	import DPad from "./Buttons/DPad.svelte";
	import GameButton from "../GameButton.svelte";
	import Scaler from "../Scaler/Scaler.svelte";
</script>

<style lang="scss">
	div {
		display: flex;
		justify-content: space-between;
	}

	.save-load {
		display: flex;
		flex-direction: column;
		gap: 8px;
		align-self: center;

		button {
			padding: 4px 16px;
		}
	}

	.speed {
		display: flex;
		gap: 4px;
		align-self: center;

		button {
			padding: 4px 8px;
		}

		button.active {
			font-weight: bold;
		}
	}
</style>