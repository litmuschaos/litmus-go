package com.github.litmuschaos.litmusgo;

import java.io.File;

import com.sun.tools.attach.VirtualMachine;

public class JvmFaultInjectorMain {

	/*
	 * Calling this will install the jar file it belongs to as a Java Agent. 
	 * This will call the agentmain method of the JvmFaultInjectorAgent class.
	 */
	public static void main(String[] args) {
		try {
			String jvmPid = args[0];
			String options = null;
			if (args.length > 1) {
				for (int i = 1; i < args.length; i++) {
					if (options == null) {
						options = args[i];
					} else {
						options = options + " " + args[i];
					}
				}
			}
			VirtualMachine jvm = VirtualMachine.attach(jvmPid);
			// Lookup of the path to the running JAR
			String jarUrl = JvmFaultInjectorMain.class.getProtectionDomain().getCodeSource().getLocation().getPath();
			String jarPath = new File(jarUrl).getCanonicalPath();
			jvm.loadAgent(jarPath, options);
			jvm.detach();
		} catch (Exception e) {
			e.printStackTrace();
		}
	}
    
}
