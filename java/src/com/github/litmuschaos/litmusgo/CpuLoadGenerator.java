package com.github.litmuschaos.litmusgo;

public class CpuLoadGenerator {
	private long fib0 = 0;
	private long fib1 = 1;
	

	public void generateLoad() {
		long fib2 = fib0 + fib1;
		fib0 = fib1;
		fib1 = fib2;
	}
}

