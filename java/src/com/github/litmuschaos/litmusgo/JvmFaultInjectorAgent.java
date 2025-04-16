package com.github.litmuschaos.litmusgo;

import java.lang.instrument.Instrumentation;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.concurrent.atomic.AtomicBoolean;

public class JvmFaultInjectorAgent {
	private static AtomicBoolean alive = new AtomicBoolean(false);

	// This method will be called when the jar is installed as an agent
	public static void agentmain(String argsString, Instrumentation inst) throws Exception {
		try {
			String[] args = {};
			if (argsString != null) {
				args = argsString.split(" ");
			}
			final List<String> argsList = Arrays.asList(args);
			String command = "";
			if (args.length > 0) {
				command = args[0];
			}
			if ("cpu".equals(command)) {
				alive.set(true);
				consumeCpu(argsList);
			} else if ("mem".equals(command)) {
				alive.set(true);
				allocateMemory(argsList);
			} else if ("stop".equals(command)) {
				alive.set(false);
			}
		} catch (Exception e) {
			e.printStackTrace();
		}
	}

	private static void consumeCpu(List<String> argsList) {
		// Total duration in seconds
		int duration = parseIntArg(argsList, "-d", 60);
		long stopTime = System.currentTimeMillis() + (duration * 1000);
		// Number of threads
		int numThreads = parseIntArg(argsList, "-t", 1);
		for (int i = 0; i < numThreads; i++) {
			new Thread(() -> {
				CpuLoadGenerator cpuLoadGenerator = new CpuLoadGenerator();
				while (alive.get() && System.currentTimeMillis() < stopTime) {
					cpuLoadGenerator.generateLoad();
				}
			}).start();
		}
	}

	private static void allocateMemory(List<String> argsList) {
		// Keep references, simulate a memory leak
		boolean keep = argsList.contains("-k");
		// Amount of megabytes to allocate in each iteration.
		int memory = parseIntArg(argsList, "-m", 1);
		if (memory > 2047) {
			// Memory parameter cannot be more than 2047 MiB. Setting it to 2047.
			memory = 2047;
		}
		int amountOfBytesToAllocate = 1024 * 1024 * memory;
		// Time to sleep for each iteration, in milliseconds
		int sleepMillis = parseIntArg(argsList, "-s", 2000);
		// Total duration in seconds
		int duration = parseIntArg(argsList, "-d", 60);
		long stopTime = System.currentTimeMillis() + (duration * 1000);
		ArrayList<ByteBuffer> keepReferences = new ArrayList<ByteBuffer>();
		new Thread(() -> {
			boolean allocateMemory = true;
			while (alive.get() && System.currentTimeMillis() < stopTime) {
				try {
					if (allocateMemory) {
						ByteBuffer bb = ByteBuffer.allocate(amountOfBytesToAllocate);
						if (keep) {
							keepReferences.add(bb);
						}
					}
					sleep(sleepMillis);
				} catch (OutOfMemoryError outOfMemoryError) {
					// If an OutOfMemory occurs, we stop allocating more memory but continue the
					// thread, keeping references until the duration is over.
					allocateMemory = false;
				}
			}
		}).start();
	}

	private static int parseIntArg(List<String> args, String argName, int defaultValue) {
		if (args.contains(argName)) {
			return Integer.parseInt(args.get(args.indexOf(argName) + 1));
		}
		return defaultValue;
	}

	private static void sleep(long time) {
		try {
			Thread.sleep(time);
		} catch (InterruptedException e) {
			e.printStackTrace();
		}
	}
}
