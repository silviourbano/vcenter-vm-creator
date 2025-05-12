package main

import (
	"context"
	"encoding/json" // For pretty printing backing info
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// Variables to be loaded from .env or environment
var (
	vmwareURL       string
	vmwareUsername  string
	vmwarePassword  string
	vCenterInsecure string
)

// Parameters
var (
	datacenterName = flag.String("datacenter", "DC-PRODUCAO", "Target Datacenter")
	clusterName    = flag.String("cluster", "PRD-DB-LINUX", "Target Cluster")
	// For optimal performance, provide the full inventory path to the template if known,
	// e.g., "/DC-PRODUCAO/vm/MyTemplateFolder/OracleDB_OL7_Template".
	// Otherwise, searching by name might take longer.
	// Para performance ótima, forneça o caminho completo do inventário para o template se conhecido,
	// ex: "/DC-PRODUCAO/vm/MinhaPastaDeTemplates/OracleDB_OL7_Template".
	// Caso contrário, a busca pelo nome pode demorar mais.
	templateName       = flag.String("template", "OracleDB_OL7_Template", "VM template name or full inventory path")
	vmName             = flag.String("vm-name", "my-new-vm", "New virtual machine name")
	datastoreName      = flag.String("datastore", "TESP5STG1P00002_VMWARE_DB_LNX_34", "Target Datastore")
	networkName        = flag.String("network", "TSILV_CSILV_ate_network-production", "Network label name")
	resourcePoolName   = flag.String("resource-pool", "", "Target Resource Pool (optional)")
	inspectVMName      = flag.String("inspect-vm", "", "Name of an existing VM to inspect its network configuration")
	targetVMFolderName = flag.String("folder", "TSILV_CSILV_ate", "Target VM Folder name (default: TSILV_CSILV_ate)")
)

func loadEnv() {
	log.Println("Attempting to load .env file...")
	err := godotenv.Load()
	if err != nil {
		log.Println("Info: .env file not found or error loading, relying on environment variables.")
	} else {
		log.Println(".env file loaded successfully.")
	}
	vmwareURL = os.Getenv("VMWARE_URL")
	vmwareUsername = os.Getenv("VMWARE_USERNAME")
	vmwarePassword = os.Getenv("VMWARE_PASSWORD")
	vCenterInsecure = os.Getenv("VCENTER_INSECURE")
	log.Printf("VMWARE_URL: %s", vmwareURL)
	log.Printf("VMWARE_USERNAME: %s", vmwareUsername)
	log.Printf("VCENTER_INSECURE: %s", vCenterInsecure)
}

func inspectExistingVMNetwork(ctx context.Context, c *govmomi.Client, dc *object.Datacenter, vmNameToInspect string) {
	log.Printf("Attempting to inspect network configuration for VM: %s", vmNameToInspect)
	finder := find.NewFinder(c.Client, true)
	finder.SetDatacenter(dc)

	existingVM, err := finder.VirtualMachine(ctx, vmNameToInspect)
	if err != nil {
		log.Fatalf("Failed to find VM '%s' for inspection: %s", vmNameToInspect, err)
	}
	log.Printf("Found VM '%s' (MOID: %s) for inspection.", existingVM.Name(), existingVM.Reference().Value)

	var vmMo mo.VirtualMachine
	pc := property.DefaultCollector(c.Client)
	err = pc.RetrieveOne(ctx, existingVM.Reference(), []string{"config.hardware.device"}, &vmMo)
	if err != nil {
		log.Fatalf("Failed to retrieve hardware devices for VM '%s': %s", existingVM.Name(), err)
	}

	log.Printf("Hardware devices for VM '%s':", existingVM.Name())
	foundNIC := false
	for _, device := range vmMo.Config.Hardware.Device {
		if nic, ok := device.(types.BaseVirtualEthernetCard); ok {
			foundNIC = true
			virtualNic := nic.GetVirtualEthernetCard()
			log.Printf("  NIC Label: %s, Device Type: %T", virtualNic.DeviceInfo.GetDescription().Label, nic)
			log.Printf("    AddressType: %s", virtualNic.AddressType)
			if virtualNic.MacAddress != "" {
				log.Printf("    MAC Address: %s", virtualNic.MacAddress)
			}
			log.Printf("    Connectable: %+v", virtualNic.Connectable)

			if virtualNic.Backing == nil {
				log.Println("    BackingInfo: nil")
				continue
			}
			log.Printf("    BackingInfo Type: %T", virtualNic.Backing)
			backingJSON, err := json.MarshalIndent(virtualNic.Backing, "    ", "  ")
			if err != nil {
				log.Printf("    Could not marshal backing info to JSON: %s", err)
				switch backing := virtualNic.Backing.(type) {
				case *types.VirtualEthernetCardNetworkBackingInfo:
					log.Printf("      DeviceName: %s", backing.DeviceName)
					if backing.Network != nil {
						log.Printf("      Network MOR: %s (Type: %s, Value: %s)", backing.Network.Type, backing.Network.Value)
					} else {
						log.Printf("      Network MOR: nil")
					}
					log.Printf("      UseAutoDetect: %t", backing.UseAutoDetect)
				case *types.VirtualEthernetCardDistributedVirtualPortBackingInfo:
					log.Printf("      Port.SwitchUuid: %s", backing.Port.SwitchUuid)
					log.Printf("      Port.PortgroupKey: %s", backing.Port.PortgroupKey)
					log.Printf("      Port.PortKey: %s", backing.Port.PortKey)
					log.Printf("      Port.ConnectionCookie: %d", backing.Port.ConnectionCookie)
				default:
					log.Printf("    BackingInfo: %+v", virtualNic.Backing)
				}
			} else {
				log.Printf("    BackingInfo (JSON):\n    %s", string(backingJSON))
			}
			log.Println("    ----")
		}
	}
	if !foundNIC {
		log.Printf("  No VirtualEthernetCard devices found on VM '%s'.", existingVM.Name())
	}
}

func main() {
	log.Println("Starting script...")
	overallStartTime := time.Now()
	loadEnv()
	flag.Parse()
	log.Println("Command line flags parsed.")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if vmwareURL == "" || vmwareUsername == "" || vmwarePassword == "" {
		log.Fatal("VMWARE_URL, VMWARE_USERNAME, and VMWARE_PASSWORD must be set.")
	}
	log.Println("Environment variables for vCenter connection seem present.")

	parsedURL, err := url.Parse(vmwareURL)
	if err != nil {
		log.Fatalf("Failed to parse VMWARE_URL: %s", err)
	}
	parsedURL.User = url.UserPassword(vmwareUsername, vmwarePassword)
	log.Printf("VMware URL parsed. Attempting connection to: %s", parsedURL.Hostname())

	insecureConnection := vCenterInsecure == "true"
	connectStartTime := time.Now()
	log.Printf("Attempting to connect to vCenter (User: %s, Insecure: %t)...", vmwareUsername, insecureConnection)
	c, err := govmomi.NewClient(ctx, parsedURL, insecureConnection)
	if err != nil {
		log.Fatalf("Failed to connect to vCenter: %s", err)
	}
	defer c.Logout(ctx)
	log.Printf("Successfully connected to vCenter: %s (took %s)", parsedURL.Hostname(), time.Since(connectStartTime))

	finder := find.NewFinder(c.Client, true)
	pc := property.DefaultCollector(c.Client)

	log.Printf("Finding Datacenter for context: %s...", *datacenterName)
	dcFindStartTime := time.Now()
	dc, err := finder.Datacenter(ctx, *datacenterName)
	if err != nil {
		log.Fatalf("Failed to find Datacenter '%s': %s", *datacenterName, err)
	}
	log.Printf("Found Datacenter: %s (MOID: %s) (took %s)", dc.Name(), dc.Reference().Value, time.Since(dcFindStartTime))
	finder.SetDatacenter(dc)

	if *inspectVMName != "" {
		inspectExistingVMNetwork(ctx, c, dc, *inspectVMName)
		log.Printf("Inspection finished in %s.", time.Since(overallStartTime))
		return
	}

	log.Println("Proceeding with VM cloning logic...")
	// Construct absolute path for the target VM folder for optimized search
	// Constrói caminho absoluto para a pasta de destino da VM para busca otimizada
	effectiveFixedVMFolderPath := fmt.Sprintf("/%s/vm/%s", *datacenterName, *targetVMFolderName)

	log.Printf("Finding Cluster: %s...", *clusterName)
	clusterFindStartTime := time.Now()
	cluster, err := finder.ClusterComputeResource(ctx, *clusterName)
	if err != nil {
		log.Fatalf("Failed to find Cluster '%s': %s", *clusterName, err)
	}
	log.Printf("Found Cluster: %s (MOID: %s) (took %s)", cluster.Name(), cluster.Reference().Value, time.Since(clusterFindStartTime))

	var rp *object.ResourcePool
	rpFindStartTime := time.Now()
	if *resourcePoolName != "" {
		log.Printf("Finding Resource Pool: %s...", *resourcePoolName)
		rp, err = finder.ResourcePool(ctx, *resourcePoolName)
		if err != nil {
			log.Fatalf("Failed to find Resource Pool '%s': %s", *resourcePoolName, err)
		}
		log.Printf("Found Resource Pool: %s (MOID: %s) (took %s)", rp.Name(), rp.Reference().Value, time.Since(rpFindStartTime))
	} else {
		log.Printf("Resource Pool name not specified, using root resource pool for Cluster '%s'...", cluster.Name())
		rp, err = cluster.ResourcePool(ctx)
		if err != nil {
			log.Fatalf("Failed to get root Resource Pool for Cluster '%s': %s", cluster.Name(), err)
		}
		log.Printf("Found root Resource Pool for Cluster '%s' (MOID: %s) (took %s)", cluster.Name(), rp.Reference().Value, time.Since(rpFindStartTime))
	}

	log.Printf("Finding Datastore: %s...", *datastoreName)
	dsFindStartTime := time.Now()
	ds, err := finder.Datastore(ctx, *datastoreName)
	if err != nil {
		log.Fatalf("Failed to find Datastore '%s': %s", *datastoreName, err)
	}
	log.Printf("Found Datastore: %s (MOID: %s) (took %s)", ds.Name(), ds.Reference().Value, time.Since(dsFindStartTime))

	log.Printf("Finding Network: %s...", *networkName)
	netFindStartTime := time.Now()
	netObj, err := finder.Network(ctx, *networkName)
	if err != nil {
		log.Fatalf("Failed to find Network '%s': %s", *networkName, err)
	}
	log.Printf("Found Network: %s (Type: %T) (took %s)", *networkName, netObj, time.Since(netFindStartTime))

	// Find VM Template using the name or path provided in the -template flag.
	// If a full inventory path (e.g., "/DC_NAME/vm/folder/template_name") is provided, search will be faster.
	// Encontra Template de VM usando o nome ou caminho fornecido na flag -template.
	// Se um caminho completo do inventário (ex: "/NOME_DC/vm/pasta/nome_template") for fornecido, a busca será mais rápida.
	log.Printf("Finding VM Template using name/path: %s...", *templateName)
	templateFindStartTime := time.Now()
	templateVM, err := finder.VirtualMachine(ctx, *templateName)
	if err != nil {
		log.Fatalf("Failed to find VM Template with name/path '%s': %s", *templateName, err)
	}
	log.Printf("Found VM Template: %s (MOID: %s) (took %s)", templateVM.Name(), templateVM.Reference().Value, time.Since(templateFindStartTime))

	log.Printf("Finding fixed VM Folder using absolute path: %s...", effectiveFixedVMFolderPath)
	folderFindStartTime := time.Now()
	vmFolder, err := finder.Folder(ctx, effectiveFixedVMFolderPath)
	if err != nil {
		log.Fatalf("Failed to find fixed VM Folder with path '%s': %s", effectiveFixedVMFolderPath, err)
	}
	log.Printf("Found fixed VM Folder: %s (MOID: %s) (took %s)", vmFolder.Name(), vmFolder.Reference().Value, time.Since(folderFindStartTime))

	relocateSpec := types.VirtualMachineRelocateSpec{
		Datastore: types.NewReference(ds.Reference()),
		Pool:      types.NewReference(rp.Reference()),
	}
	log.Println("Relocation spec configured.")

	cloneSpec := types.VirtualMachineCloneSpec{
		Location: relocateSpec,
		PowerOn:  false,
		Template: false,
	}
	log.Println("Initial clone spec configured.")

	log.Println("Retrieving hardware devices from template for network configuration...")
	var templateMo mo.VirtualMachine
	retrieveHwStartTime := time.Now()
	err = pc.RetrieveOne(ctx, templateVM.Reference(), []string{"config.hardware.device", "network"}, &templateMo)
	if err != nil {
		log.Fatalf("Failed to retrieve template hardware devices: %s (took %s)", err, time.Since(retrieveHwStartTime))
	}
	log.Printf("Retrieved %d hardware devices from template (took %s).", len(templateMo.Config.Hardware.Device), time.Since(retrieveHwStartTime))

	var deviceChanges []types.BaseVirtualDeviceConfigSpec
	networkConfigStartTime := time.Now()

	dvpg, ok := netObj.(*object.DistributedVirtualPortgroup)
	if !ok {
		log.Fatalf("Network '%s' is not a DistributedVirtualPortgroup, but type %T. This script currently only supports DVPG for network modification.", *networkName, netObj)
	}

	var dvpgMo mo.DistributedVirtualPortgroup
	err = pc.RetrieveOne(ctx, dvpg.Reference(), []string{"key", "config.distributedVirtualSwitch"}, &dvpgMo)
	if err != nil {
		log.Fatalf("Failed to retrieve properties for DVPG '%s': %s", dvpg.Name(), err)
	}
	if dvpgMo.Config.DistributedVirtualSwitch == nil {
		log.Fatalf("DVPG '%s' does not have an associated DistributedVirtualSwitch in its config property.", dvpg.Name())
	}

	dvsObject := object.NewDistributedVirtualSwitch(c.Client, *dvpgMo.Config.DistributedVirtualSwitch)
	var dvsMo mo.DistributedVirtualSwitch
	err = pc.RetrieveOne(ctx, dvsObject.Reference(), []string{"summary.uuid"}, &dvsMo)
	if err != nil {
		log.Fatalf("Failed to retrieve DVS summary for UUID for DVS '%s': %s", dvsObject.Reference().Value, err)
	}
	dvsUuid := dvsMo.Summary.Uuid
	portgroupKey := dvpgMo.Key
	log.Printf("DVPG Key: %s, DVS UUID: %s for network %s", portgroupKey, dvsUuid, dvpg.Name())

	for _, device := range templateMo.Config.Hardware.Device {
		if nic, ok := device.(types.BaseVirtualEthernetCard); ok {
			templateNic := nic.GetVirtualEthernetCard()

			log.Printf("  Template NIC Label: %s, Original BackingInfo Type: %T", templateNic.DeviceInfo.GetDescription().Label, templateNic.Backing)
			// Optional: Log originalBackingJSON if needed for debugging
			// if templateNic.Backing != nil {
			// 	originalBackingJSON, _ := json.MarshalIndent(templateNic.Backing, "    ", "  ")
			// 	log.Printf("    Original BackingInfo (JSON):\n    %s", string(originalBackingJSON))
			// }

			log.Printf("  Attempting to configure NIC (Key: %d, Label: %s) for network '%s'.", templateNic.Key, templateNic.DeviceInfo.GetDescription().Label, *networkName)

			newBackingInfo := &types.VirtualEthernetCardDistributedVirtualPortBackingInfo{
				Port: types.DistributedVirtualSwitchPortConnection{
					SwitchUuid:   dvsUuid,
					PortgroupKey: portgroupKey,
				},
			}

			var editedNicDevice types.BaseVirtualDevice
			switch nic.(type) {
			case *types.VirtualVmxnet3:
				editedNicDevice = &types.VirtualVmxnet3{
					VirtualVmxnet: types.VirtualVmxnet{
						VirtualEthernetCard: types.VirtualEthernetCard{
							VirtualDevice: types.VirtualDevice{
								Key:         templateNic.Key,
								Backing:     newBackingInfo,
								Connectable: templateNic.Connectable,
							},
							AddressType: templateNic.AddressType,
						},
					},
				}
			case *types.VirtualE1000:
				editedNicDevice = &types.VirtualE1000{
					VirtualEthernetCard: types.VirtualEthernetCard{
						VirtualDevice: types.VirtualDevice{
							Key:         templateNic.Key,
							Backing:     newBackingInfo,
							Connectable: templateNic.Connectable,
						},
						AddressType: templateNic.AddressType,
					},
				}
			case *types.VirtualE1000e:
				editedNicDevice = &types.VirtualE1000e{
					VirtualEthernetCard: types.VirtualEthernetCard{
						VirtualDevice: types.VirtualDevice{
							Key:         templateNic.Key,
							Backing:     newBackingInfo,
							Connectable: templateNic.Connectable,
						},
						AddressType: templateNic.AddressType,
					},
				}
			case *types.VirtualPCNet32:
				editedNicDevice = &types.VirtualPCNet32{
					VirtualEthernetCard: types.VirtualEthernetCard{
						VirtualDevice: types.VirtualDevice{
							Key:         templateNic.Key,
							Backing:     newBackingInfo,
							Connectable: templateNic.Connectable,
						},
						AddressType: templateNic.AddressType,
					},
				}
			default:
				log.Printf("  WARNING: Unsupported NIC type %T found in template. Attempting generic VirtualEthernetCard for edit.", nic)
				editedNicDevice = &types.VirtualEthernetCard{
					VirtualDevice: types.VirtualDevice{
						Key:         templateNic.Key,
						Backing:     newBackingInfo,
						Connectable: templateNic.Connectable,
					},
					AddressType: templateNic.AddressType,
				}
			}

			deviceConfigSpec := &types.VirtualDeviceConfigSpec{
				Operation: types.VirtualDeviceConfigSpecOperationEdit,
				Device:    editedNicDevice,
			}
			deviceChanges = append(deviceChanges, deviceConfigSpec)
			log.Printf("  NIC (Key: %d) prepared for connection to DVS UUID '%s', PortgroupKey '%s' using minimal edit spec.", templateNic.Key, dvsUuid, portgroupKey)
		}
	}
	log.Printf("Network adapter configuration processed (took %s).", time.Since(networkConfigStartTime))

	if len(deviceChanges) > 0 {
		cloneSpec.Config = &types.VirtualMachineConfigSpec{
			DeviceChange: deviceChanges,
		}
		log.Printf("%d network device(s) prepared for modification in clone spec.", len(deviceChanges))
	} else {
		log.Println("No VirtualEthernetCard devices found in the template. Skipping network device modification for clone.")
	}

	log.Printf("Starting clone operation for VM '%s' from template '%s' into folder '%s'...", *vmName, templateVM.Name(), vmFolder.Name())
	cloneTaskStartTime := time.Now()
	task, err := templateVM.Clone(ctx, vmFolder, *vmName, cloneSpec)
	if err != nil {
		log.Fatalf("Failed to initiate clone VM task: %s", err)
	}
	log.Printf("Clone task submitted (Task MOID: %s). Waiting for result...", task.Reference().Value)

	taskWaitStartTime := time.Now()
	info, err := task.WaitForResult(ctx, nil)
	if err != nil {
		if info != nil && info.Error != nil {
			log.Fatalf("Clone task failed: %s. Reason from vCenter: %s (Task MOID: %s)", err, info.Error.LocalizedMessage, task.Reference().Value)
		}
		log.Fatalf("Clone task failed: %s (Task MOID: %s)", err, task.Reference().Value)
	}
	log.Printf("Clone task completed (took %s for waiting).", time.Since(taskWaitStartTime))

	clonedVMRef, ok := info.Result.(types.ManagedObjectReference)
	if !ok {
		log.Fatalf("Clone task result is not a ManagedObjectReference, got: %T", info.Result)
	}
	clonedVM := object.NewVirtualMachine(c.Client, clonedVMRef)

	log.Printf("Successfully cloned VM: %s (MOID: %s) into folder '%s'. Total clone operation took %s.",
		*vmName, clonedVM.Reference().Value, vmFolder.Name(), time.Since(cloneTaskStartTime))

	log.Printf("Script finished successfully in %s.", time.Since(overallStartTime))
}
