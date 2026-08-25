<div class="agent-panel">
	<div class="heading">
		<span class="name">Agent</span>
		<span class="status" class:idle={$agentState.status === AgentStatus.Idle} class:running={$agentState.status === AgentStatus.Running} class:paused={$agentState.status === AgentStatus.Paused} class:error={$agentState.status === AgentStatus.Error}>
			{$agentState.status}
		</span>
	</div>
	<div class="list-content">
		<ul>
			<li>
				<span class="list-prefix">Step</span>
				<span>{$agentState.step}</span>
			</li>
			<li>
				<span class="list-prefix">Goal</span>
				<span>{$agentState.goal}</span>
			</li>
			<li>
				<span class="list-prefix">Last Action</span>
				<span>{$agentState.last_action}</span>
			</li>
			<li>
				<span class="list-prefix">Observation</span>
				<span>{$agentState.observation}</span>
			</li>
		</ul>
	</div>
</div>

<script>
	import Game, { AgentStatus } from "$lib/game";

	let { agentState } = Game;
</script>

<style lang="scss">
	.agent-panel {
		border-radius: 4px;
		display: grid;
		font-family: "Roboto", "Ubuntu", "sans-serif";
		overflow: hidden;
	}
	.heading {
		background-color: #000066;
		display: flex;
		font-size: 15px;
		line-height: 25px;

		.name {
			color: #fff;
			flex-grow: 1;
			padding: 0 8px;
		}

		.status {
			background-color: #666666;
			color: #fff;
			padding: 0 8px;
		}

		.status.running {
			background-color: #006600;
		}

		.status.paused {
			background-color: #666600;
		}

		.status.error {
			background-color: #660000;
		}
	}
	.list-content {
		background-color: #003300;
		display: flex;
		font-weight: 300;
		padding: 2px 8px;

		ul {
			display: grid;
			gap: 2px;
		}

		li {
			background-color: transparent;
			display: flex;
		}
	}
	.list-prefix {
		font-weight: 700;
		margin-right: 4px;

		&+span {
			flex-grow: 1;
		}
	}
</style>
